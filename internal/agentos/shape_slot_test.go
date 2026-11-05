package agentos

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/shape/slot"
)

func withShapeSlotSeams(t *testing.T) {
	t.Helper()
	oldRunning, oldExe, oldSelect, oldLaunch, oldApp, oldHostApp, oldRelease, oldOwned := RunningAsBashy, shapeSlotExecutable, shapeSlotSelect, shapeSlotLaunch, shapeSlotApp, shapeSlotHostApp, shapeSlotRelease, shapeSlotOwnedImage
	t.Cleanup(func() {
		RunningAsBashy = oldRunning
		shapeSlotExecutable = oldExe
		shapeSlotSelect = oldSelect
		shapeSlotLaunch = oldLaunch
		shapeSlotApp = oldApp
		shapeSlotHostApp = oldHostApp
		shapeSlotRelease = oldRelease
		shapeSlotOwnedImage = oldOwned
	})
	RunningAsBashy = true
	shapeSlotHostApp = func(string) bool { return false }
	shapeSlotOwnedImage = func(string) bool { return false }
	t.Setenv("VSC_PROFILE", "")
	t.Setenv(slot.EntryEnvironmentKey, "")
}
func TestShapeSlotProductDispatchPreservesInvocation(t *testing.T) {
	withShapeSlotSeams(t)
	dir := t.TempDir()
	release := filepath.Join(dir, "released")
	active := filepath.Join(dir, "shaped")
	shapeSlotExecutable = func() (string, error) { return release, nil }
	shapeSlotSelect = func(app string) (slot.Version, bool, error) {
		if app != "bashy" {
			t.Fatalf("app=%q", app)
		}
		return slot.Version{Path: active}, true, nil
	}
	args := []string{"bashy", "shape", "status", "hello-world", "--json"}
	launched := false
	shapeSlotLaunch = func(path string, actualArgs, env []string) int {
		launched = true
		if path != active || !reflect.DeepEqual(actualArgs, args) {
			t.Fatal("selected path/argv changed")
		}
		marker := false
		for _, entry := range env {
			if entry == slot.EntryEnvironmentKey+"="+active {
				marker = true
			}
		}
		if !marker {
			t.Fatal("launch lacks image-bound recursion marker")
		}
		return 7
	}
	code, handled := maybeRunShapedBashy(args)
	if !handled || code != 7 || !launched {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
}
func TestShapeSlotRecoveryAlwaysUsesRelease(t *testing.T) {
	withShapeSlotSeams(t)
	shapeSlotSelect = func(string) (slot.Version, bool, error) {
		t.Fatal("recovery consulted active executable")
		return slot.Version{}, false, nil
	}
	for _, args := range [][]string{
		{"bashy", "shape", "status", "bashy"},
		{"bashy", "shape", "--user", "principal", "undo", "bashy"},
		{"bashy", "shape", "restore", "--as=actor", "bashy", "release"},
		{"bashy", "shape", "--json", "status", "bashy"},
	} {
		if _, handled := maybeRunShapedBashy(args); handled {
			t.Fatalf("recovery redirected: %v", args)
		}
	}
	if shapeReleasedControl([]string{"bashy", "shape", "--user", "undo", "show", "hello-world", "candidate"}) {
		t.Fatal("flag value treated as recovery verb")
	}
}
func TestShapeSlotPinnedPreviewDoesNotRedirect(t *testing.T) {
	withShapeSlotSeams(t)
	preview := filepath.Join(t.TempDir(), "preview")
	shapeSlotExecutable = func() (string, error) { return preview, nil }
	t.Setenv(slot.EntryEnvironmentKey, preview)
	shapeSlotSelect = func(string) (slot.Version, bool, error) {
		t.Fatal("pinned preview consulted active host slot")
		return slot.Version{}, false, nil
	}
	if _, handled := maybeRunShapedBashy([]string{"bashy", "shape", "--help"}); handled {
		t.Fatal("preview redirected")
	}
}
func TestShapeSlotInheritedMarkerDoesNotPinAnotherExecutable(t *testing.T) {
	withShapeSlotSeams(t)
	dir := t.TempDir()
	original, active := filepath.Join(dir, "original"), filepath.Join(dir, "active")
	t.Setenv(slot.EntryEnvironmentKey, filepath.Join(dir, "previous-parent-image"))
	shapeSlotExecutable = func() (string, error) { return original, nil }
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{Path: active}, true, nil }
	shapeSlotLaunch = func(string, []string, []string) int { return 0 }
	if _, handled := maybeRunShapedBashy([]string{"bashy", "shape", "status", "hello-world"}); !handled {
		t.Fatal("parent marker pinned a different executable")
	}
}
func TestShapeSlotMissingOrInvalidSelectionKeepsInstallation(t *testing.T) {
	withShapeSlotSeams(t)
	shapeSlotExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "released"), nil }
	for _, err := range []error{nil, errors.New("digest mismatch")} {
		shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{}, false, err }
		if _, handled := maybeRunShapedBashy([]string{"bashy", "--version"}); handled {
			t.Fatal("invalid or absent slot redirected")
		}
	}
}
func TestShapeSlotNeverChangesPureShellOrCertificationEntry(t *testing.T) {
	withShapeSlotSeams(t)
	shapeSlotSelect = func(string) (slot.Version, bool, error) {
		t.Fatal("non-product entry consulted slot")
		return slot.Version{}, false, nil
	}
	RunningAsBashy = false
	if _, handled := maybeRunShapedBashy([]string{"sh", "-c", "echo hi"}); handled {
		t.Fatal("pure shell redirected")
	}
	RunningAsBashy = true
	if _, handled := maybeRunShapedBashy([]string{"bashy", "runner", "--model", "synthetic"}); handled {
		t.Fatal("embedded inference runner redirected")
	}
	t.Setenv("VSC_PROFILE", "cert")
	if _, handled := maybeRunShapedBashy([]string{"bashy", "-c", "echo hi"}); handled {
		t.Fatal("certification entry redirected")
	}
}
func TestShapeSlotRegisteredCLIChangesOnlyExecutable(t *testing.T) {
	withShapeSlotSeams(t)
	shapeSlotApp = func(name string) bool { return name == "example" }
	selected := filepath.Join(t.TempDir(), "example-shaped")
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{Path: selected}, true, nil }
	original := []string{"released-program", "fixed-argument", "user-argument"}
	argv, err := shapedRegisteredArgv("example", original)
	if err != nil {
		t.Fatal(err)
	}
	if argv[0] != selected || !reflect.DeepEqual(argv[1:], original[1:]) || original[0] != "released-program" {
		t.Fatal("registered arguments mutated")
	}
	argv, err = shapedRegisteredArgv("ordinary", original)
	if err != nil || !reflect.DeepEqual(argv, original) {
		t.Fatal("ordinary registered command changed")
	}
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{}, false, errors.New("invalid digest") }
	if _, err = shapedRegisteredArgv("example", original); err == nil {
		t.Fatal("invalid artifact selected")
	}
}
func TestShapeSlotEntryEnvironmentReplacesOnlyMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary")
	env, err := slot.EntryEnvironment(path, []string{"OTHER=value", slot.EntryEnvironmentKey + "=old"})
	if err != nil {
		t.Fatal(err)
	}
	markers := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, slot.EntryEnvironmentKey+"=") {
			markers++
		}
	}
	if markers != 1 || env[0] != "OTHER=value" {
		t.Fatalf("environment=%v", env)
	}
}

func TestShapeSlotUntrustedInitCannotReceiveControlPlane(t *testing.T) {
	withShapeSlotSeams(t)
	dir := t.TempDir()
	binary := filepath.Join(dir, "shape-init")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./testdata/shape-init/main.go")
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build init fixture: %v %s", err, out)
	}
	log := filepath.Join(dir, "initialized")
	t.Setenv("BASHY_SHAPE_INIT_FIXTURE_LOG", log)
	shapeSlotExecutable = func() (string, error) { return filepath.Join(dir, "released"), nil }
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{Path: binary}, true, nil }
	shapeSlotLaunch = launchShapeSlot
	for _, args := range [][]string{
		{"bashy", "shape", "wish", "hello-world", "change"},
		{"bashy", "shape", "verify", "hello-world", "candidate"},
		{"bashy", "shape", "approve", "hello-world", "candidate"},
		{"bashy", "shape", "submit", "hello-world", "revision"},
		{"bashy", "shape", "preview", "hello-world", "candidate"},
		{"bashy", "mcp", "serve", "--tools", "shape"},
		{"bashy", "src", "activate", "hello-world", "revision"},
		{"bashy", "app", "add", "hello-world"},
		{"bashy", "service", "restart", "example"},
	} {
		if _, handled := maybeRunShapedBashy(args); handled {
			t.Fatalf("control plane redirected: %v", args)
		}
		if _, err := os.Stat(log); !os.IsNotExist(err) {
			t.Fatal("untrusted init ran before released control plane")
		}
	}
	for _, args := range [][]string{{"bashy", "shape", "--help"}, {"bashy", "shape", "status", "hello-world"}} {
		if code, handled := maybeRunShapedBashy(args); !handled || code != 0 {
			t.Fatalf("read-only/help did not use shaped executable: %v code=%d", args, code)
		}
		if _, err := os.Stat(log); err != nil {
			t.Fatal("shaped init fixture did not execute")
		}
		if err := os.Remove(log); err != nil {
			t.Fatal(err)
		}
	}
}

func TestShapeSlotSelectedImageDelegatesControlToRetainedRelease(t *testing.T) {
	withShapeSlotSeams(t)
	dir := t.TempDir()
	active, release := filepath.Join(dir, "active"), filepath.Join(dir, "released")
	shapeSlotExecutable = func() (string, error) { return active, nil }
	shapeSlotOwnedImage = func(string) bool { return true }
	shapeSlotRelease = func(string) (slot.Version, bool, error) { return slot.Version{Path: release}, true, nil }
	var launched string
	shapeSlotLaunch = func(path string, args, env []string) int { launched = path; return 0 }
	if code, handled := maybeRunShapedBashy([]string{"bashy", "shape", "approve", "hello-world", "candidate"}); !handled || code != 0 || launched != release {
		t.Fatal("selected executable retained shaping authority")
	}
	shapeSlotExecutable = func() (string, error) { return release, nil }
	if _, handled := maybeRunShapedBashy([]string{"bashy", "shape", "verify", "hello-world", "candidate"}); handled {
		t.Fatal("retained control plane recursively delegated")
	}
}

func TestShapeSlotMCPToolDispatchKeepsReleasedControl(t *testing.T) {
	withShapeSlotSeams(t)
	ringDir(t)
	shapeSlotExecutable = func() (string, error) { return filepath.Join(t.TempDir(), "released"), nil }
	shapeSlotSelect = func(string) (slot.Version, bool, error) {
		t.Fatal("MCP authority consulted selected image")
		return slot.Version{}, false, nil
	}
	if _, handled := maybeRunShapedBashy([]string{"bashy", "mcp", "serve", "--tools", "shape"}); handled {
		t.Fatal("MCP server startup selected a shaped implementation")
	}
	oldExec := mcpVerbExec
	defer func() { mcpVerbExec = oldExec }()
	calls := 0
	mcpVerbExec = func(_ context.Context, argv []string, stdin, dir string) (string, string, int) {
		calls++
		if !shapeReleasedControl(argv) {
			t.Fatalf("MCP callback lost control-plane classification: %v", argv)
		}
		if _, handled := maybeRunShapedBashy(argv); handled {
			t.Fatal("MCP callback selected native shadow implementation")
		}
		return `{"released":true}`, "", 0
	}
	ctx, client, _ := mcpFrontClient(t, mcpVerbOptions(t, "destroy,spend", "all"))
	for _, verb := range []string{"verify", "approve", "submit"} {
		result := mcpSessionCall(t, ctx, client, "shape", map[string]any{"args": []string{verb, "hello-world", "candidate", "--json"}})
		out := mcpVerbDecode(t, result)
		if result.IsError || out.ExitCode != 0 {
			t.Fatalf("MCP %s failed", verb)
		}
	}
	if calls != 3 {
		t.Fatalf("actual MCP tool callbacks=%d", calls)
	}
}
func TestShapeSlotShellEntriesStayReleasedAndFrontendRemainsShapeable(t *testing.T) {
	withShapeSlotSeams(t)
	for _, args := range [][]string{
		{"bashy"}, {"bashy", "-c", "shape approve hello-world candidate"}, {"bashy", "-s"}, {"bashy", "--bashsharp", "example.bsh"}, {"bashy", "./script"}, {"bashy", "example.sh"},
	} {
		if !shapeReleasedControl(args) {
			t.Fatalf("shell entry can select native controller: %v", args)
		}
	}
	for _, args := range [][]string{{"bashy", "--help"}, {"bashy", "--version"}, {"bashy", "handoff", "show", "story:example"}} {
		if shapeReleasedControl(args) {
			t.Fatalf("read-only/new frontend forced released: %v", args)
		}
	}
}

func TestShapeSlotReleaseSourceRequiresCleanEmbeddedProvenance(t *testing.T) {
	old := shapeReleaseBuildInfo
	defer func() { shapeReleaseBuildInfo = old }()
	actual := strings.Repeat("a", 40)
	t.Setenv("BASHY_SRC_COMMIT", strings.Repeat("b", 40))
	shapeReleaseBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: actual}, {Key: "vcs.modified", Value: "false"}}}, true
	}
	if got := shapeKnownReleaseRevision(); got != actual {
		t.Fatal("development override replaced actual embedded revision")
	}
	shapeReleaseBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: actual}, {Key: "vcs.modified", Value: "true"}}}, true
	}
	if shapeKnownReleaseRevision() != "" {
		t.Fatal("dirty bytes claimed exact release source")
	}
	shapeReleaseBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: actual}}, Deps: []*debug.Module{{Path: "example.invalid/dependency", Replace: &debug.Module{Path: "../dependency", Version: "(devel)"}}}}, true
	}
	if shapeKnownReleaseRevision() != "" {
		t.Fatal("live workspace dependency claimed pinned source proof")
	}
	shapeReleaseBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	if shapeKnownReleaseRevision() != "" {
		t.Fatal("missing build metadata claimed exact source")
	}
}
