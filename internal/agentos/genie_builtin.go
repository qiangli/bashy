package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a
//
// genie is bashy's builtin agent: its source (ycode/examples/genie) is linked
// into this binary, so `bashy genie` on a host with only bashy builds its own
// bundle on first use — no ycode checkout, no separate ycode, no released
// bundle to fetch. A bundle built from a checkout (`bashy genie build --from`,
// or an auto-discovered checkout) is a developer's own workspace override: it
// is kept only for a caller running from inside that same directory
// (dirProvenanceScoped), never as a new host default for every other caller.
// A bundle built from the builtin source is rebuilt when this bashy carries a
// different source. Story b24f3f95: a worker's own `genie build` had been
// silently adopted by every later invocation on the host, past a builtin
// source change, because the old check kept any "dir" provenance forever.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	geniesrc "github.com/qiangli/ycode/examples/genie"
)

// genieProvenance records where the installed bundle came from, next to it.
type genieProvenance struct {
	Source string `json:"source"`           // "builtin" or "dir"
	Digest string `json:"digest,omitempty"` // builtin: the embedded source digest
	Path   string `json:"path,omitempty"`   // dir: the source directory
}

func genieProvenancePath(home string) string { return filepath.Join(home, "genie.source.json") }

func readGenieProvenance(home string) (genieProvenance, bool) {
	var p genieProvenance
	data, err := os.ReadFile(genieProvenancePath(home))
	if err != nil || json.Unmarshal(data, &p) != nil {
		return genieProvenance{}, false
	}
	return p, true
}

func writeGenieProvenance(home string, p genieProvenance) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(genieProvenancePath(home), append(data, '\n'), 0o644)
}

// builtinGenieDigest names the embedded source: sha256 over each file's path
// and content, in path order.
func builtinGenieDigest() (string, error) {
	var paths []string
	err := fs.WalkDir(geniesrc.Source, ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		data, err := geniesrc.Source.ReadFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", path, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// materializeBuiltinGenie writes the embedded source to home/src/<digest>
// (once; a finished tree carries a marker) and returns that directory.
func materializeBuiltinGenie(home, digest string) (string, error) {
	dir := filepath.Join(home, "src", digest[:16])
	marker := filepath.Join(dir, ".complete")
	if _, err := os.Stat(marker); err == nil {
		return dir, nil
	}
	tmp, err := os.MkdirTemp(filepath.Join(home), "src-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	err = fs.WalkDir(geniesrc.Source, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(tmp, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := geniesrc.Source.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmp, ".complete"), []byte(digest+"\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	_ = os.RemoveAll(dir) // an unfinished tree from an interrupted run
	if err := os.Rename(tmp, dir); err != nil {
		return "", err
	}
	return dir, nil
}

// buildBuiltinGenie builds the bundle from the builtin source. A variable so
// tests can stand in for the real build, which runs this very binary (`bashy
// dag`) — inside a test that is the test binary, which would run itself.
var buildBuiltinGenie = installBuiltinGenie

// installBuiltinGenie packages the embedded source and installs the bundle.
func installBuiltinGenie(home string, stderr io.Writer) error {
	digest, err := builtinGenieDigest()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return err
	}
	source, err := materializeBuiltinGenie(home, digest)
	if err != nil {
		return err
	}
	return packageGenie(source, genieProvenance{Source: "builtin", Digest: digest}, stderr)
}

// builtinGenieCurrent reports whether an installed bundle may be used as is:
// a developer's build (`build --from`, recorded as "dir") only while this
// invocation belongs to that same directory (dirProvenanceScoped); a builtin
// build only while it matches this bashy's embedded source. A bundle with no
// provenance predates the builtin source and is rebuilt from it. A
// dir-sourced bundle is a per-workspace override, never the host default: a
// caller outside that directory falls back to (and rebuilds) the builtin
// bundle instead of silently inheriting another workspace's build forever.
func builtinGenieCurrent(home string) bool {
	p, ok := readGenieProvenance(home)
	if !ok {
		return false
	}
	if p.Source != "builtin" {
		return dirProvenanceScoped(p.Path)
	}
	digest, err := builtinGenieDigest()
	return err == nil && p.Digest == digest
}

// dirProvenanceScoped reports whether the current invocation belongs to the
// directory a `genie build --from DIR` bundle was built from: the caller
// names it again with GENIE_SOURCE, or is running from inside it (at or below
// it). Any other caller treats the bundle as stale.
func dirProvenanceScoped(path string) bool {
	if source := strings.TrimSpace(os.Getenv("GENIE_SOURCE")); source != "" {
		abs, err := filepath.Abs(source)
		if err != nil {
			abs = source
		}
		return resolvePath(abs) == resolvePath(path)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	return pathUnder(path, cwd)
}

// resolvePath evaluates symlinks (e.g. macOS's /tmp -> /private/tmp) so a
// path survives round-tripping through os.Getwd; it falls back to the
// cleaned input when the path does not exist or cannot be resolved.
func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// pathUnder reports whether target is root itself or nested under it.
func pathUnder(root, target string) bool {
	root, target = resolvePath(root), resolvePath(target)
	if root == target {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
