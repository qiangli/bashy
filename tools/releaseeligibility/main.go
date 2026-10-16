// Command releaseeligibility rejects diagnostic Bashy build profiles before
// an artifact is promoted to a shipped path, and verifies a release asset
// directory carries the managed otel blobs binmgr fetches on demand. Signal
// structure is audited separately by tools/elfaudit.
package main

import (
	"debug/buildinfo"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/qiangli/yoke/external/loom"
	"github.com/qiangli/yoke/external/seaweedfs"
	"github.com/qiangli/yoke/external/zot"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// otelPlatforms mirrors the PLATFORMS matrix in scripts/publish-otel-blob.sh:
// every bashy release must carry one otel-<goos>-<goarch>.gz per entry,
// because binmgr fetches that exact asset name from the latest release on
// the first `bashy otel serve`.
var otelPlatforms = []string{
	"darwin/arm64",
	"darwin/amd64",
	"linux/amd64",
	"linux/arm64",
	"windows/amd64",
}

// requiredOtelAssets returns the release asset names the otel stack needs.
func requiredOtelAssets() []string {
	names := make([]string, 0, len(otelPlatforms))
	for _, p := range otelPlatforms {
		goos, goarch, _ := strings.Cut(p, "/")
		names = append(names, "otel-"+goos+"-"+goarch+".gz")
	}
	return names
}

// checkReleaseAssets flags a release asset directory that is missing any otel
// blob. Presence, not exact set: sibling assets are QA'd by
// scripts/verify-release-asset-set.sh.
func checkReleaseAssets(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read release asset directory %s: %w", dir, err)
	}
	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		present[e.Name()] = true
	}
	var missing []string
	for _, want := range requiredOtelAssets() {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s is missing otel release blobs: %s", dir, strings.Join(missing, ", "))
	}
	return nil
}

// capability is a release-required engine/service and its pinned binmgr tool.
type capability struct {
	name string
	tool binmgr.Tool
}

// requiredCapabilities reuses the yoke binmgr pins as the single source of
// truth for which engines/services ship and what they are pinned to.
func requiredCapabilities() []capability {
	return []capability{
		{"prima", binmgr.PrimaTool()},
		{"zot", zot.ZotTool},
		{"gitea/loom", loom.LoomTool},
		{"seaweedfs", seaweedfs.SeaweedfsTool},
	}
}

// checkSignedAssets requires, for every capability and every supported OS
// platform (windows included), a pinned asset that is signed: a Sigstore
// identity on the tool plus a committed digest or a per-asset bundle.
func checkSignedAssets(caps []capability) error {
	var problems []string
	for _, c := range caps {
		for _, plat := range otelPlatforms {
			goos, _, _ := strings.Cut(plat, "/")
			a, ok := c.tool.Assets[plat]
			switch {
			case !ok || a.URL == "":
				problems = append(problems, fmt.Sprintf("%s: no signed asset for %s (%s)", c.name, goos, plat))
			case c.tool.Sigstore == nil || (a.SHA256 == "" && a.SigstoreBundle == "" && !pinned(c.tool, plat)):
				problems = append(problems, fmt.Sprintf("%s: asset %s for %s (%s) is unsigned/unpinned", c.name, a.URL, goos, plat))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("release-required assets missing:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

func pinned(t binmgr.Tool, plat string) bool {
	_, ok := binmgr.PinnedSHA256(t.Name, t.Version, plat)
	return ok
}

// checkTierVerdict requires a PASS dhnt.tier-smoke/v1 record for commit.
func checkTierVerdict(path, commit string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tier verdict record: %w", err)
	}
	var rec struct {
		Schema  string `json:"schema"`
		Verdict string `json:"verdict"`
		Commit  string `json:"commit"`
	}
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("parse tier verdict record %s: %w", path, err)
	}
	if rec.Schema != "dhnt.tier-smoke/v1" {
		return fmt.Errorf("tier verdict %s: schema %q, want dhnt.tier-smoke/v1", path, rec.Schema)
	}
	if rec.Commit != commit {
		return fmt.Errorf("tier verdict %s is for commit %q, candidate is %q", path, rec.Commit, commit)
	}
	if rec.Verdict != "PASS" {
		return fmt.Errorf("tier verdict for %s is %q, want PASS", commit, rec.Verdict)
	}
	return nil
}

func main() {
	signed := flag.Bool("signed-assets", false, "verify every release-required capability pins a signed asset for every supported OS")
	tier := flag.String("tier-verdict", "", "path to the dhnt.tier-smoke/v1 verdict record; requires --commit")
	commit := flag.String("commit", "", "candidate commit the tier verdict must name")
	assets := flag.String("assets", "", "verify release asset directory DIR carries the required otel blobs")
	flag.Parse()
	if *signed {
		if err := checkSignedAssets(requiredCapabilities()); err != nil {
			fmt.Fprintf(os.Stderr, "releaseeligibility: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("releaseeligibility: PASS signed assets")
		return
	}
	if *tier != "" {
		if *commit == "" {
			fmt.Fprintln(os.Stderr, "usage: releaseeligibility --tier-verdict FILE --commit SHA")
			os.Exit(2)
		}
		if err := checkTierVerdict(*tier, *commit); err != nil {
			fmt.Fprintf(os.Stderr, "releaseeligibility: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("releaseeligibility: PASS tier verdict %s\n", *commit)
		return
	}
	if *assets != "" {
		if flag.NArg() != 0 {
			fmt.Fprintln(os.Stderr, "usage: releaseeligibility --assets DIR")
			os.Exit(2)
		}
		if err := checkReleaseAssets(*assets); err != nil {
			fmt.Fprintf(os.Stderr, "releaseeligibility: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("releaseeligibility: PASS %s (otel assets)\n", *assets)
		return
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: releaseeligibility PATH")
		os.Exit(2)
	}
	if err := audit(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "releaseeligibility: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("releaseeligibility: PASS %s\n", flag.Arg(0))
}

func audit(path string) error {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Go build information from %s: %w", path, err)
	}
	if build.Path != "github.com/qiangli/bashy/cmd/bashy" {
		return fmt.Errorf("%s contains %q, want cmd/bashy", path, build.Path)
	}
	tags := make(map[string]bool)
	settings := make(map[string]string)
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
		if setting.Key == "-tags" {
			for _, tag := range strings.FieldsFunc(setting.Value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
				tags[tag] = true
			}
		}
	}
	if tags["bashy_core"] {
		return fmt.Errorf("%s uses diagnostic bashy_core tag; do not ship until optional command parity and route gates pass", path)
	}
	if tags["bashy_cert_base"] && (!tags["bashy_cert"] || !tags["osusergo"] || settings["GOOS"] != "linux" || settings["CGO_ENABLED"] != "1") {
		return fmt.Errorf("%s uses bashy_cert_base without Linux CGO and osusergo certification tags and settings", path)
	}
	return nil
}
