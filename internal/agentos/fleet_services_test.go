// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
)

func mustFleetServicesDir(t *testing.T) string {
	t.Helper()
	root, ok := findBashySourceRoot(mustGetwd())
	if !ok {
		t.Fatal("could not find bashy source root")
	}
	servicesDir := filepath.Join(root, "examples", "fleet", "services")
	if fi, err := os.Stat(servicesDir); err != nil || !fi.IsDir() {
		t.Fatalf("examples/fleet/services dir not found at %s: %v", servicesDir, err)
	}
	return servicesDir
}

// TestFleetServicesYAMLHeadersAndSecretsGuard asserts:
// - Every services/*.yaml header comment has `# status:`, `# add:`, `# source:` lines.
// - Both record the verified monitor and paired mesh gate.
// - No host paths or secret tokens are leaked.
func TestFleetServicesYAMLHeadersAndSecretsGuard(t *testing.T) {
	servicesDir := mustFleetServicesDir(t)
	files, err := filepath.Glob(filepath.Join(servicesDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no service files found under %s: %v", servicesDir, err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 service files, found %d", len(files))
	}

	reUsers := regexp.MustCompile(`/Users/`)
	reLocal := regexp.MustCompile(`\b[a-zA-Z0-9_\-\.]+\.local\b`)
	reSKToken := regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{10,}`)

	for _, f := range files {
		base := filepath.Base(f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: read file: %v", base, err)
		}
		content := string(data)
		if !strings.Contains(content, "# status:") {
			t.Errorf("%s: missing `# status:` line in header comment", base)
		}
		if !strings.Contains(content, "# add:") {
			t.Errorf("%s: missing `# add:` line in header comment", base)
		}
		if !strings.Contains(content, "# source:") {
			t.Errorf("%s: missing `# source:` line in header comment", base)
		}
		if !strings.Contains(content, "# status:  verified") {
			t.Errorf("%s: status must record the verified live gate", base)
		}
		if match := reUsers.FindString(content); match != "" {
			t.Errorf("%s: contains real path pattern %q", base, match)
		}
		if match := reLocal.FindString(content); match != "" {
			t.Errorf("%s: contains .local hostname %q", base, match)
		}
		if match := reSKToken.FindString(content); match != "" {
			t.Errorf("%s: contains API key token pattern %q", base, match)
		}
	}
}

// TestFleetServicesParseAndValidate verifies that both third-party service recipes
// parse cleanly via fleet.ParseService and pass fleet.Service.Validate().
func TestFleetServicesParseAndValidate(t *testing.T) {
	servicesDir := mustFleetServicesDir(t)
	files, err := filepath.Glob(filepath.Join(servicesDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no service files found: %v", err)
	}

	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(base, ".yaml")
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: read: %v", base, err)
		}
		svc, err := fleet.ParseService(name, data, nil)
		if err != nil {
			t.Fatalf("%s: ParseService failed: %v", base, err)
		}
		if err := svc.Validate(); err != nil {
			t.Errorf("%s: Validate failed: %v", base, err)
		}
		if svc.Name != name {
			t.Errorf("%s: Name = %q, want %q", base, svc.Name, name)
		}
		if svc.Kind != fleet.KindService {
			t.Errorf("%s: Kind = %q, want %q", base, svc.Kind, fleet.KindService)
		}
		if svc.ProviderName() != fleet.ServiceProviderProcess {
			t.Errorf("%s: Provider = %q, want %q", base, svc.ProviderName(), fleet.ServiceProviderProcess)
		}
	}
}

// TestFleetServicesOfflineRegistryLifecycle exercises service add, show, enable,
// disable and rm in an isolated store without needing a live supervisor.
func TestFleetServicesOfflineRegistryLifecycle(t *testing.T) {
	servicesDir := mustFleetServicesDir(t)
	files, err := filepath.Glob(filepath.Join(servicesDir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no service files: %v", err)
	}

	for _, f := range files {
		base := filepath.Base(f)
		name := strings.TrimSuffix(base, ".yaml")
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("BASHY_HOME", home)
			t.Setenv("BASHY_SERVICES_DIR", filepath.Join(home, "services"))
			t.Setenv("BASHY_SERVICES_PATH", "")

			// 1. Add unit
			addOut := svcRun(t, "add", "-f", f)
			if !strings.Contains(addOut, name) {
				t.Errorf("add output %q did not mention %q", addOut, name)
			}

			// 2. Show --json round-trip
			showJSON := svcRun(t, "show", "--json", name)
			var showObj map[string]any
			if err := json.Unmarshal([]byte(showJSON), &showObj); err != nil {
				t.Fatalf("show --json invalid: %v\n%s", err, showJSON)
			}
			if showObj["name"] != name {
				t.Errorf("show name = %v, want %q", showObj["name"], name)
			}

			// 3. Enable unit (desired state)
			svcRun(t, "enable", name)
			cat := fleet.New()
			s, ok := cat.Service(name)
			if !ok {
				t.Fatalf("service %s not found in catalog after enable", name)
			}
			if !s.Enabled {
				t.Errorf("service %s Enabled = false after enable", name)
			}

			// 4. Disable unit
			svcRun(t, "disable", name)
			cat = fleet.New()
			s, ok = cat.Service(name)
			if !ok || s.Enabled {
				t.Errorf("service %s Enabled = %v after disable, want false", name, s.Enabled)
			}

			// 5. Remove unit
			svcRun(t, "rm", name)
			cat = fleet.New()
			if _, ok := cat.Service(name); ok {
				t.Errorf("service %s still in catalog after rm", name)
			}
		})
	}
}

// TestCodyncHostPinnedDownloadDeclaration validates the official Codync release pin
// and declared ports, health probe, and mesh reach.
func TestCodyncHostPinnedDownloadDeclaration(t *testing.T) {
	servicesDir := mustFleetServicesDir(t)
	f := filepath.Join(servicesDir, "codync-host.yaml")
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := fleet.ParseService("codync-host", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Validate(); err != nil {
		t.Fatal(err)
	}

	dl := svc.Download
	if dl == nil {
		t.Fatal("codync-host must declare download")
	}
	if dl.GitHub != "leepokai/Codync" {
		t.Errorf("download.github = %q, want leepokai/Codync", dl.GitHub)
	}
	if dl.Version != "v2.13.2" {
		t.Errorf("download.version = %q, want v2.13.2", dl.Version)
	}
	if dl.Member != "codync-host" {
		t.Errorf("download.member = %q, want codync-host", dl.Member)
	}

	expectedDigests := map[string]string{
		"darwin/arm64": "7c59d120f81b5bd582ec1340cbf775d390a2fb434d8d47d7104887e4a2e9d621",
		"darwin/amd64": "0a0d642f2119f4b5362f5330c78c92dac5c512970643884c36c35bcbe46251c9",
		"linux/arm64":  "39449983897d520f00c62ceaf911cbd435fc04a317cfef23cacfa59eb6e133b3",
		"linux/amd64":  "422d342f1369f0a9ebf3943ff2e5a8b5aba0382809d10d211daae289d3b320a4",
	}
	for plat, wantDigest := range expectedDigests {
		got, ok := dl.SHA256[plat]
		if !ok {
			t.Errorf("missing sha256 for platform %s", plat)
		} else if got != wantDigest {
			t.Errorf("sha256[%s] = %q, want %q", plat, got, wantDigest)
		}
	}

	// ExpandExec verification
	argv, err := svc.ExpandExec("/test/path/codync-host")
	if err != nil {
		t.Fatalf("ExpandExec failed: %v", err)
	}
	if len(argv) != 6 || argv[0] != "/test/path/codync-host" || argv[1] != "serve" || argv[3] != "19222" || argv[4] != "--bind" || argv[5] != "127.0.0.1" {
		t.Errorf("ExpandExec argv = %v, want loopback-bound Codync host", argv)
	}

	// Health check
	if !svc.Health.TCP || svc.Health.Port != "host" {
		t.Errorf("health = %+v, want TCP probe on port host", svc.Health)
	}

	// Expose mesh
	if len(svc.Expose.Mesh) == 0 || svc.Expose.Mesh[0].Name != "codync" {
		t.Errorf("mesh expose = %+v, want name codync", svc.Expose.Mesh)
	}
}

// TestOpenClawGatewayOCILauncherDeclaration validates the generic OCI launcher recipe,
// pinned OCI image digest, declared health probe (/health), secrets, and tunnel/mesh exposure.
func TestOpenClawGatewayOCILauncherDeclaration(t *testing.T) {
	servicesDir := mustFleetServicesDir(t)
	f := filepath.Join(servicesDir, "openclaw-gateway.yaml")
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := fleet.ParseService("openclaw-gateway", data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Validate(); err != nil {
		t.Fatal(err)
	}

	// Generic OCI launcher invocation in foreground
	if len(svc.Exec) == 0 || (svc.Exec[0] != "podman" && svc.Exec[0] != "docker") {
		t.Fatalf("exec[0] = %v, want generic OCI launcher (podman or docker)", svc.Exec)
	}
	if svc.Exec[1] != "run" {
		t.Errorf("exec[1] = %q, want run", svc.Exec[1])
	}

	// Verified digests recorded
	wantIndexDigest := "sha256:d0ded1dd76939b2bf4d67ef2d13247b8b160aa5666331d4a0b0e58811182cbb8"
	wantAmd64 := "sha256:5139eb3ca71ba20d3cd3aa933338e63dbff857d384e482fbdb7dd557849e5529"
	wantArm64 := "sha256:d9af04f6ff74936e0dab1a780251f89a85ae70189529c8139eaf5e1c6be534fb"

	if svc.Settings["image_digest"] != wantIndexDigest {
		t.Errorf("settings.image_digest = %q, want %q", svc.Settings["image_digest"], wantIndexDigest)
	}
	if svc.Settings["linux_amd64"] != wantAmd64 {
		t.Errorf("settings.linux_amd64 = %q, want %q", svc.Settings["linux_amd64"], wantAmd64)
	}
	if svc.Settings["linux_arm64"] != wantArm64 {
		t.Errorf("settings.linux_arm64 = %q, want %q", svc.Settings["linux_arm64"], wantArm64)
	}

	// Exec argv contains the pinned image digest
	execJoined := strings.Join(svc.Exec, " ")
	if !strings.Contains(execJoined, wantIndexDigest) {
		t.Errorf("exec argv %q does not contain pinned image digest %s", execJoined, wantIndexDigest)
	}

	// ExpandExec verification
	argv, err := svc.ExpandExec("")
	if err != nil {
		t.Fatalf("ExpandExec failed: %v", err)
	}
	argvJoined := strings.Join(argv, " ")
	if !strings.Contains(argvJoined, "127.0.0.1:18789:18789") {
		t.Errorf("expanded argv %q missing port mapping 127.0.0.1:18789:18789", argvJoined)
	}

	// Health check: /health on gateway port
	if svc.Health.HTTP != "/health" || svc.Health.Port != "gateway" {
		t.Errorf("health = %+v, want HTTP /health probe on port gateway", svc.Health)
	}

	// Secrets: vault refs for bot tokens
	secretsMap := map[string]string{}
	for _, sec := range svc.Secrets {
		secretsMap[sec.Env] = sec.Ref
	}
	if secretsMap["OPENCLAW_GATEWAY_TOKEN"] != "openclaw/gateway-token" {
		t.Errorf("missing OPENCLAW_GATEWAY_TOKEN secret ref")
	}
	if _, ok := secretsMap["TELEGRAM_BOT_TOKEN"]; ok {
		t.Error("optional Telegram token must not block base gateway start")
	}
	if _, ok := secretsMap["DISCORD_BOT_TOKEN"]; ok {
		t.Error("optional Discord token must not block base gateway start")
	}

	// Expose: app tunnel + mesh
	if svc.Expose.App == nil || svc.Expose.App.Name != "openclaw" {
		t.Errorf("expose.app = %+v, want openclaw", svc.Expose.App)
	}
	if len(svc.Expose.Mesh) == 0 || svc.Expose.Mesh[0].Name != "openclaw-gateway" {
		t.Errorf("expose.mesh = %+v, want openclaw-gateway", svc.Expose.Mesh)
	}
}
