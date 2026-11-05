package agentos

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/shape/slot"
	"github.com/qiangli/yoke/pkg/src"
)

var shapeSlotExecutable = os.Executable
var shapeSlotSelect = func(app string) (slot.Version, bool, error) { return (slot.Store{Home: src.Home(), App: app}).Select() }
var shapeSlotRelease = func(app string) (slot.Version, bool, error) {
	return (slot.Store{Home: src.Home(), App: app}).RetainedRelease()
}
var shapeSlotOwnedImage = func(executable string) bool {
	root, err := (slot.Store{Home: src.Home(), App: "bashy"}).Dir()
	if err != nil {
		return false
	}
	root = filepath.Join(root, "artifacts")
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if real, err := filepath.EvalSymlinks(executable); err == nil {
		executable = real
	}
	relative, err := filepath.Rel(root, executable)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
var shapeSlotLaunch = launchShapeSlot
var shapeSlotHostApp = func(name string) bool {
	app, ok := fleet.New().App(name)
	return ok && app.Shaping != nil && (app.Shaping.Kind == "cli" || app.Shaping.Kind == "service")
}
var shapeSlotApp = func(name string) bool {
	app, ok := fleet.New().App(name)
	return ok && app.Shaping != nil && app.Shaping.Kind == "cli"
}

// shapeReleasedControl keeps shaping authority on the installed control plane.
// Approved native code still shares the local OS account's authority; this is
// a routing boundary, not a sandbox for malicious approved executables.
func shapeReleasedControl(args []string) bool {
	if len(args) < 2 {
		return true // bare product invocation is the released interactive shell
	}
	entry := args[1]
	if strings.HasPrefix(entry, "-") && entry != "--help" && entry != "--version" {
		return true
	}
	switch strings.ToLower(filepath.Ext(entry)) {
	case ".sh", ".bash", ".bsh", ".bpp":
		return true
	}
	if strings.ContainsAny(entry, "/\\") {
		return true
	}
	if st, err := os.Stat(entry); err == nil && st.Mode().IsRegular() && !isFrontDoorInvocation(entry) {
		return true
	}
	switch args[1] {
	case "mcp", "src", "app", "apps", "service", "supervisord", "self", "bootstrap", "upgrade":
		return true
	case "shape":
	default:
		return false
	}
	var positional []string
	takesValue := map[string]bool{"--user": true, "--principal": true, "--as": true, "--agent": true, "--remote": true, "--cookie-file": true, "--console": true, "--story": true, "--todo": true, "--from": true, "--page": true, "--element": true, "--kind": true, "--path": true, "--id": true, "--to": true, "--contract": true}
	for i := 2; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			key, _, hasValue := strings.Cut(a, "=")
			if takesValue[key] && !hasValue {
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	if len(positional) == 0 {
		return false
	} // root help is shapeable
	switch positional[0] {
	case "status", "show", "diff", "history", "suggest":
		if len(positional) < 2 {
			return false
		}
		return positional[1] == "bashy" || shapeSlotHostApp(positional[1])
	default:
		return true // mutations and unknown future authority stay released
	}
}

// maybeRunShapedBashy is wired only at the product's AgentOS entry. The pure
// shell aliases, applet aliases and embedded inference runner do not reach it.
func maybeRunShapedBashy(args []string) (int, bool) {
	if !RunningAsBashy || os.Getenv("VSC_PROFILE") == "cert" || (len(args) > 1 && args[1] == "runner") {
		return 0, false
	}
	exe, err := shapeSlotExecutable()
	if err != nil {
		return 0, false
	}
	if shapeReleasedControl(args) {
		if !slot.PinnedEntry(exe) && !shapeSlotOwnedImage(exe) {
			return 0, false
		}
		release, found, err := shapeSlotRelease("bashy")
		if err != nil || !found {
			fmt.Fprintln(os.Stderr, "bashy: retained released control plane unavailable for this selected image")
			return 126, true
		}
		if sameShapeExecutable(exe, release.Path) {
			return 0, false
		}
		env, err := slot.EntryEnvironment(release.Path, os.Environ())
		if err != nil {
			return 126, true
		}
		return shapeSlotLaunch(release.Path, args, env), true
	}
	if slot.PinnedEntry(exe) {
		return 0, false
	}
	active, found, err := shapeSlotSelect("bashy")
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy: shaped executable unavailable; using retained installation:", err)
		return 0, false
	}
	if !found || sameShapeExecutable(exe, active.Path) {
		return 0, false
	}
	env, err := slot.EntryEnvironment(active.Path, os.Environ())
	if err != nil {
		return 0, false
	}
	return shapeSlotLaunch(active.Path, args, env), true
}
func sameShapeExecutable(a, b string) bool {
	first, e1 := os.Stat(a)
	second, e2 := os.Stat(b)
	if e1 == nil && e2 == nil {
		return os.SameFile(first, second)
	}
	firstPath, _ := filepath.Abs(a)
	secondPath, _ := filepath.Abs(b)
	return firstPath == secondPath
}

func launchShapeSlot(path string, args, env []string) int {
	tail := []string{}
	if len(args) > 1 {
		tail = args[1:]
	}
	cmd := exec.Command(path, tail...)
	if len(args) > 0 {
		cmd.Args[0] = args[0]
	}
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if e := cmd.Run(); e != nil {
		if exit, ok := e.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "bashy: could not start shaped executable:", e)
		return 126
	}
	return 0
}

// shapedRegisteredArgv changes only the executable selected by an explicitly
// registered CLI-kind app. Existing fixed arguments and user arguments remain.
func shapedRegisteredArgv(name string, argv []string) ([]string, error) {
	if len(argv) == 0 || os.Getenv("VSC_PROFILE") == "cert" || !shapeSlotApp(name) {
		return argv, nil
	}
	active, ok, e := shapeSlotSelect(name)
	if e != nil {
		return nil, e
	}
	if !ok {
		return argv, nil
	}
	out := append([]string(nil), argv...)
	out[0] = active.Path
	return out, nil
}

// shapeCLIRelease resolves the actual retained/installed executable. It never
// infers installed source identity from an arbitrary app-source directory.
func shapeCLIRelease(name string) (path, revision string, err error) {
	release, found, err := (slot.Store{Home: src.Home(), App: name}).RetainedRelease()
	if err != nil {
		return "", "", err
	}
	if found {
		revision = release.Revision
		if revision == "unknown" {
			revision = ""
		}
		return release.Path, revision, nil
	}
	if name == "bashy" {
		path, err = shapeSlotExecutable()
		return path, shapeKnownReleaseRevision(), err
	}
	rec, ok := registeredLookup(name)
	if !ok || len(rec.Exec) == 0 {
		return "", "", nil
	}
	path, err = exec.LookPath(rec.Exec[0])
	return path, "", err
}

// shapeKnownReleaseRevision is deliberately stricter than src.ReleaseCommit:
// a development override is declared metadata, not evidence about these bytes.
var shapeReleaseBuildInfo = debug.ReadBuildInfo

func shapeKnownReleaseRevision() string {
	info, ok := shapeReleaseBuildInfo()
	if !ok {
		return ""
	}
	revision := ""
	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			return ""
		}
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
	}
	bytes, err := hex.DecodeString(revision)
	if err != nil || len(bytes) != 20 {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Replace != nil && (dep.Replace.Version == "" || dep.Replace.Version == "(devel)") {
			return ""
		}
	}
	return revision
}
