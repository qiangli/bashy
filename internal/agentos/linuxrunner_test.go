// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16le is what Windows console tools actually write, even redirected.
func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// fakeHost answers the probe/step commands a test declares and records every
// command line it was asked to run.
type fakeHost struct {
	replies map[string]struct {
		out  []byte
		exit int
		err  error
	}
	calls []string
}

func newFakeHost() *fakeHost {
	return &fakeHost{replies: map[string]struct {
		out  []byte
		exit int
		err  error
	}{}}
}

// reply keys on a prefix of the joined command line, so a test declares only
// as much of the argv as identifies the command.
func (f *fakeHost) reply(prefix string, out []byte, exit int, err error) *fakeHost {
	f.replies[prefix] = struct {
		out  []byte
		exit int
		err  error
	}{out, exit, err}
	return f
}

func (f *fakeHost) run(_ context.Context, name string, args ...string) ([]byte, int, error) {
	line := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, line)
	best := ""
	for prefix := range f.replies {
		if strings.HasPrefix(line, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return nil, 1, nil // an undeclared command simply fails
	}
	r := f.replies[best]
	return r.out, r.exit, r.err
}

func (f *fakeHost) ran(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func fakeEnv(t *testing.T, goos string, h *fakeHost, present ...string) (linuxEnv, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	cwd := `C:\Users\liqia\src\cloudbox`
	if goos != "windows" {
		cwd = "/home/liqiang/src/cloudbox"
	}
	return linuxEnv{
		goos:   goos,
		goarch: "amd64",
		self:   "/bin/bashy",
		cwd:    cwd,
		run:    h.run,
		look: func(name string) (string, error) {
			for _, p := range present {
				if p == name {
					return `C:\Windows\System32\` + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		stdout: &out,
		stderr: &errOut,
	}, &out, &errOut
}

// ---------------------------------------------------------------- decoding

func TestDecodeConsole(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []byte
		want string
	}{
		{"utf8", []byte("Ubuntu-24.04\n"), "Ubuntu-24.04\n"},
		{"utf16le no bom", utf16le("Ubuntu-24.04\n"), "Ubuntu-24.04\n"},
		{"utf16le bom", append([]byte{0xFF, 0xFE}, utf16le("State : Enabled\n")...), "State : Enabled\n"},
		{"empty", nil, ""},
		// The real sentence wsl.exe prints when the component is absent; this
		// is the exact text that read as "W i n d o w s" before the decoder.
		{"wsl not installed", utf16le("The Windows Subsystem for Linux is not installed.\r\n"),
			"The Windows Subsystem for Linux is not installed.\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := decodeConsole(tc.in); got != tc.want {
				t.Errorf("decodeConsole = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDISMState(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"\r\nFeature Information:\r\n\r\nFeature Name : VirtualMachinePlatform\r\nState : Disabled\r\n", "Disabled"},
		{"State : Enabled\r\n", "Enabled"},
		{"State : Enable Pending\r\n", "Enabled"},
		{"no state here", ""},
	} {
		if got := parseDISMState(tc.in); got != tc.want {
			t.Errorf("parseDISMState(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseRegSZAndWSLDistros(t *testing.T) {
	reg := "\r\nHKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows NT\\CurrentVersion\r\n    EditionID    REG_SZ    Professional\r\n\r\n"
	if got := parseRegSZ(reg, "EditionID"); got != "Professional" {
		t.Errorf("parseRegSZ = %q, want Professional", got)
	}
	if got := parseRegSZ(reg, "Missing"); got != "" {
		t.Errorf("parseRegSZ(Missing) = %q, want empty", got)
	}
	// `wsl --list --quiet` with the component absent prints a sentence, not a
	// list; a name with spaces must never be mistaken for a distribution.
	if got := parseWSLDistros("The Windows Subsystem for Linux is not installed.\r\n"); len(got) != 0 {
		t.Errorf("parseWSLDistros(sentence) = %v, want none", got)
	}
	got := parseWSLDistros("Ubuntu-24.04\r\ndocker-desktop\r\n\r\n")
	if len(got) != 2 || got[0] != "Ubuntu-24.04" || got[1] != "docker-desktop" {
		t.Errorf("parseWSLDistros = %v", got)
	}
}

func TestParseSystemInfoVirt(t *testing.T) {
	// elephant's real answer: a hypervisor is already running, which proves
	// the firmware allows virtualization without listing the requirements.
	hv, fw := parseSystemInfoVirt("Hyper-V Requirements:          A hypervisor has been detected. Features required for Hyper-V will not be displayed.\r\n")
	if !hv || fw != "yes" {
		t.Errorf("hypervisor=%t firmware=%q, want true/yes", hv, fw)
	}
	hv, fw = parseSystemInfoVirt("Hyper-V Requirements:      VM Monitor Mode Extensions: Yes\r\n                           Virtualization Enabled In Firmware: No\r\n")
	if hv || fw != "no" {
		t.Errorf("hypervisor=%t firmware=%q, want false/no", hv, fw)
	}
	if hv, fw = parseSystemInfoVirt("nothing relevant"); hv || fw != "" {
		t.Errorf("hypervisor=%t firmware=%q, want false/unknown", hv, fw)
	}
}

// ---------------------------------------------------------------- probe

// elephantHost is the LIVE 2026-10-28 state of elephant: Windows 11 Pro
// (build 26300) with wsl.exe present, the WSL component NOT installed, both
// optional features Disabled, and a hypervisor already running.
func elephantHost() *fakeHost {
	h := newFakeHost()
	h.reply("net.exe session", nil, 0, nil)
	h.reply(`reg.exe query HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion /v EditionID`,
		[]byte("\r\n    EditionID    REG_SZ    Professional\r\n"), 0, nil)
	h.reply("wsl.exe --status", utf16le("The Windows Subsystem for Linux is not installed.\r\n"), 50, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform",
		[]byte("State : Disabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux",
		[]byte("State : Disabled\r\n"), 0, nil)
	h.reply("systeminfo.exe", []byte("Hyper-V Requirements:  A hypervisor has been detected.\r\n"), 0, nil)
	return h
}

func TestProbeElephant(t *testing.T) {
	h := elephantHost()
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	f := probeLinuxHost(context.Background(), env)

	if !f.WSLStub || f.WSLInstalled {
		t.Errorf("want stub present and component absent, got stub=%t installed=%t", f.WSLStub, f.WSLInstalled)
	}
	if f.WindowsEdition != "Professional" || !f.Elevated {
		t.Errorf("edition=%q elevated=%t", f.WindowsEdition, f.Elevated)
	}
	for _, name := range linuxWindowsFeatures {
		if f.WindowsFeatures[name] != "Disabled" {
			t.Errorf("feature %s = %q, want Disabled", name, f.WindowsFeatures[name])
		}
	}
	if !f.Hypervisor || f.FirmwareVirt != "yes" {
		t.Errorf("hypervisor=%t firmware=%q", f.Hypervisor, f.FirmwareVirt)
	}
	// wsl is offered because the stub is there; podman is not, because its
	// Windows machine needs the same virtualization that is still off.
	if !sliceHas(f.Providers, linuxProviderWSL) || sliceHas(f.Providers, linuxProviderPodman) {
		t.Errorf("providers = %v, want wsl and not podman", f.Providers)
	}
	// The probe must never try to change anything.
	for _, call := range h.calls {
		for _, forbidden := range []string{"/Enable-Feature", "--install", "shutdown", "--update", "--set-default-version"} {
			if strings.Contains(call, forbidden) {
				t.Errorf("probe ran a mutating command: %q", call)
			}
		}
	}
}

func TestProbeSkipsSystemInfoWhenFeaturesAreOn(t *testing.T) {
	h := elephantHost()
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	probeLinuxHost(context.Background(), env)
	if h.ran("systeminfo.exe") {
		t.Error("systeminfo ran even though both features are already enabled")
	}
}

func TestProbeUnelevatedLeavesFeaturesUnknown(t *testing.T) {
	h := elephantHost()
	h.reply("net.exe session", []byte("Access is denied."), 5, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	f := probeLinuxHost(context.Background(), env)
	if f.Elevated {
		t.Error("elevated reported without administrator rights")
	}
	if f.WindowsFeatures != nil {
		t.Errorf("features = %v, want unknown (nil) without dism access", f.WindowsFeatures)
	}
	if h.ran("dism.exe") {
		t.Error("dism ran unelevated; it would only produce a misleading error")
	}
}

func TestProbeLinuxHostNative(t *testing.T) {
	h := newFakeHost().reply("/bin/bashy podman info", nil, 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)
	f := probeLinuxHost(context.Background(), env)
	if !sliceHas(f.Providers, linuxProviderNative) || !f.Podman {
		t.Errorf("providers=%v podman=%t", f.Providers, f.Podman)
	}
	if got, err := selectLinuxProvider(f, ""); err != nil || got != linuxProviderNative {
		t.Errorf("selectLinuxProvider = %q, %v; want native", got, err)
	}
	// A native host IS the Linux environment: nothing to stand up, and run
	// goes straight to the command.
	plan := planLinuxRunner(env, f, "")
	if !plan.Ready || len(plan.Steps) != 0 {
		t.Errorf("native plan: ready=%t steps=%d", plan.Ready, len(plan.Steps))
	}
}

// The podman provider must not claim readiness on the strength of a reachable
// engine alone: without the image there is nothing to run the command in.
func TestPlanPodmanNeedsTheImage(t *testing.T) {
	image := selfImageTag("9.9.9", "amd64", nil)
	h := newFakeHost().reply("/bin/bashy podman info", nil, 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)
	f := probeLinuxHost(context.Background(), env)
	f.Image, f.ImagePresent = image, false

	plan := planLinuxRunner(env, f, linuxProviderPodman)
	if plan.Ready || len(plan.pending()) != 1 || plan.pending()[0].ID != "image" {
		t.Fatalf("ready=%t pending=%v", plan.Ready, plan.pending())
	}
	// Neither pending podman step may need administrator rights or a restart:
	// building an image is ordinary work.
	for _, s := range plan.Steps {
		if s.Privileged || s.Reboot {
			t.Errorf("podman step %s claims privilege (admin=%t reboot=%t)", s.ID, s.Privileged, s.Reboot)
		}
	}
	f.ImagePresent = true
	if plan := planLinuxRunner(env, f, linuxProviderPodman); !plan.Ready {
		t.Errorf("a host with the image is still not ready: %v", plan.pending())
	}
}

// A dev build cannot name its own release image, so it must say so instead of
// planning against a tag that cannot exist.
func TestPlanPodmanBlockedWithoutAVersionedImage(t *testing.T) {
	h := newFakeHost().reply("/bin/bashy podman info", nil, 0, nil)
	env, _, _ := fakeEnv(t, "linux", h)
	f := probeLinuxHost(context.Background(), env)
	f.Image, f.ImagePresent = "", false
	plan := planLinuxRunner(env, f, linuxProviderPodman)
	if !strings.Contains(plan.Blocked, "--image") || plan.Ready {
		t.Errorf("blocked=%q ready=%t", plan.Blocked, plan.Ready)
	}
}

// ---------------------------------------------------------------- provider

func TestSelectLinuxProvider(t *testing.T) {
	f := linuxFacts{OS: "windows", Arch: "amd64", Providers: []string{linuxProviderWSL}}
	if got, err := selectLinuxProvider(f, ""); err != nil || got != linuxProviderWSL {
		t.Errorf("default = %q, %v", got, err)
	}
	// A pinned provider this host cannot offer is an error, never a silent
	// fallback: a gate's result is only comparable when the venue is known.
	if _, err := selectLinuxProvider(f, linuxProviderPodman); err == nil {
		t.Error("pinning an unavailable provider did not fail")
	}
	if _, err := selectLinuxProvider(linuxFacts{OS: "windows"}, ""); err == nil {
		t.Error("a host with no provider did not fail")
	}
}

// ---------------------------------------------------------------- plan

func TestPlanElephantIsTheLiveFixture(t *testing.T) {
	h := elephantHost()
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")

	if plan.Provider != linuxProviderWSL || plan.Blocked != "" || plan.Ready {
		t.Fatalf("provider=%q blocked=%q ready=%t", plan.Provider, plan.Blocked, plan.Ready)
	}
	var ids []string
	for _, s := range plan.pending() {
		ids = append(ids, s.ID)
	}
	want := []string{
		"feature:VirtualMachinePlatform",
		"feature:Microsoft-Windows-Subsystem-Linux",
		"restart", "wsl-component", "wsl-default-version", "distro", "toolchain",
	}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("pending steps = %v\nwant %v", ids, want)
	}
	// Exactly the three host-changing steps carry the flags that stop `up`.
	for _, s := range plan.Steps {
		priv := strings.HasPrefix(s.ID, "feature:") || s.ID == "restart" || s.ID == "wsl-component"
		if s.Privileged != priv {
			t.Errorf("step %s privileged=%t, want %t", s.ID, s.Privileged, priv)
		}
		reboot := strings.HasPrefix(s.ID, "feature:") || s.ID == "restart"
		if s.Reboot != reboot {
			t.Errorf("step %s reboot=%t, want %t", s.ID, s.Reboot, reboot)
		}
		// Only one step restarts the host; the feature steps use /NoRestart
		// and merely require that it happen.
		if want := s.ID == "restart"; s.Restarts != want {
			t.Errorf("step %s restarts=%t, want %t", s.ID, s.Restarts, want)
		}
	}
}

func TestPlanConvergedWSLHostIsANoOp(t *testing.T) {
	h := elephantHost()
	h.reply("wsl.exe --status", utf16le("Default Version: 2\r\n"), 0, nil)
	h.reply("wsl.exe --list --quiet", utf16le("Ubuntu-24.04\r\n"), 0, nil)
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- sh -lc command -v go", []byte("/root/.bashy/go/bin/go\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, out, _ := fakeEnv(t, "windows", h, "wsl.exe")

	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	if !plan.Ready || len(plan.pending()) != 0 {
		t.Fatalf("ready=%t pending=%d", plan.Ready, len(plan.pending()))
	}
	if code := linuxUp(context.Background(), env, plan, linuxOptions{}); code != 0 {
		t.Fatalf("up on a converged host exited %d", code)
	}
	if !strings.Contains(out.String(), "nothing to do") {
		t.Errorf("up did not report a no-op: %q", out.String())
	}
}

// An already-installed WSL proves the optional features are on even when the
// probe could not read dism, so an unelevated converged host is still a no-op
// for the privileged steps.
func TestPlanTrustsInstalledWSLOverUnknownFeatures(t *testing.T) {
	h := elephantHost()
	h.reply("net.exe session", nil, 5, nil)
	h.reply("wsl.exe --status", utf16le("Default Version: 2\r\n"), 0, nil)
	h.reply("wsl.exe --list --quiet", utf16le("Ubuntu-24.04\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	for _, s := range plan.Steps {
		if s.Privileged && !s.Done {
			t.Errorf("privileged step %s is pending on a host where WSL already runs", s.ID)
		}
	}
}

func TestPlanBlockedWithoutWSLStub(t *testing.T) {
	env, _, _ := fakeEnv(t, "windows", elephantHost()) // no wsl.exe on PATH
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	if plan.Blocked == "" || plan.Ready {
		t.Fatalf("blocked=%q ready=%t", plan.Blocked, plan.Ready)
	}
}

func TestPlanBlockedWhenFirmwareVirtualizationIsOff(t *testing.T) {
	h := elephantHost()
	h.reply("systeminfo.exe", []byte("Virtualization Enabled In Firmware: No\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	if !strings.Contains(plan.Blocked, "firmware") {
		t.Fatalf("blocked = %q, want a firmware blocker", plan.Blocked)
	}
	// A blocker is not something `up` may try to work around.
	if code := linuxUp(context.Background(), env, plan, linuxOptions{allowPrivileged: true, allowReboot: true}); code == 0 {
		t.Error("up succeeded on a firmware-blocked host")
	}
}

// ---------------------------------------------------------------- up guards

func TestUpRefusesPrivilegedStepWithoutPermission(t *testing.T) {
	h := elephantHost()
	env, _, errOut := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")

	code := linuxUp(context.Background(), env, plan, linuxOptions{})
	if code != linuxRefusedStatus {
		t.Fatalf("up exited %d, want %d", code, linuxRefusedStatus)
	}
	msg := errOut.String()
	for _, want := range []string{
		"feature:VirtualMachinePlatform", "administrator rights",
		"dism.exe /online /Enable-Feature /FeatureName:VirtualMachinePlatform /All /NoRestart",
		"Nothing was changed", "--allow-privileged",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal missing %q:\n%s", want, msg)
		}
	}
	// The refusal must be a refusal: nothing was run on the host.
	for _, call := range h.calls {
		if strings.Contains(call, "/Enable-Feature") || strings.Contains(call, "shutdown") {
			t.Errorf("up ran %q despite refusing", call)
		}
	}
}

func TestUpRefusesRebootWithoutPermission(t *testing.T) {
	h := elephantHost()
	env, _, errOut := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	code := linuxUp(context.Background(), env, plan, linuxOptions{allowPrivileged: true})
	if code != linuxRefusedStatus {
		t.Fatalf("up exited %d, want %d", code, linuxRefusedStatus)
	}
	if !strings.Contains(errOut.String(), "--allow-reboot") {
		t.Errorf("refusal did not name --allow-reboot:\n%s", errOut.String())
	}
	if h.ran("dism.exe /online /Enable-Feature") {
		t.Error("up enabled a feature before the reboot authority was given")
	}
}

func TestUpRunsFeatureStepsAndStopsAtTheRestart(t *testing.T) {
	h := elephantHost()
	h.reply("dism.exe /online /Enable-Feature", []byte("The operation completed successfully.\r\n"), 3010, nil)
	h.reply("shutdown.exe /r", nil, 0, nil)
	env, out, _ := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")

	code := linuxUp(context.Background(), env, plan, linuxOptions{allowPrivileged: true, allowReboot: true})
	if code != 0 {
		t.Fatalf("up exited %d:\n%s", code, out.String())
	}
	for _, name := range linuxWindowsFeatures {
		if !h.ran("dism.exe /online /Enable-Feature /FeatureName:" + name) {
			t.Errorf("feature %s was not enabled", name)
		}
	}
	if !h.ran("shutdown.exe /r") {
		t.Error("the restart step did not run")
	}
	// After a restarting step, nothing else may be attempted this boot.
	if h.ran("wsl.exe --update") || h.ran("wsl.exe --install") {
		t.Error("up continued past the restart")
	}
	if !strings.Contains(out.String(), "re-run `bashy linux up`") {
		t.Errorf("up did not say how to finish:\n%s", out.String())
	}
}

func TestUpSurfacesAFailedStep(t *testing.T) {
	h := elephantHost()
	h.reply("dism.exe /online /Enable-Feature", utf16le("Error: 50\r\n"), 50, nil)
	env, _, errOut := fakeEnv(t, "windows", h, "wsl.exe")
	plan := planLinuxRunner(env, probeLinuxHost(context.Background(), env), "")
	if code := linuxUp(context.Background(), env, plan, linuxOptions{allowPrivileged: true, allowReboot: true}); code != 1 {
		t.Fatalf("up exited %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "Error: 50") {
		t.Errorf("failure did not carry the host's own message:\n%s", errOut.String())
	}
}

// ---------------------------------------------------------------- run argv

func TestWSLPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`C:\Users\liqia\src\cloudbox`, "/mnt/c/Users/liqia/src/cloudbox"},
		{`D:\`, "/mnt/d"},
		{`C:/Users/liqia`, "/mnt/c/Users/liqia"},
		{"/already/linux", "/already/linux"},
	} {
		got, err := wslPath(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("wslPath(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := wslPath(`relative\path`); err == nil {
		t.Error("a relative path was translated instead of refused")
	}
}

func TestLinuxRunArgv(t *testing.T) {
	gate := []string{"go", "test", "-short", "./..."}

	winEnv, _, _ := fakeEnv(t, "windows", newFakeHost(), "wsl.exe")
	got, err := linuxRunArgv(winEnv, linuxProviderWSL, "", gate)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"wsl.exe", "-d", "Ubuntu-24.04", "-u", "root", "--cd",
		"/mnt/c/Users/liqia/src/cloudbox", "--", "go", "test", "-short", "./..."}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("wsl argv =\n%v\nwant\n%v", got, want)
	}

	linEnv, _, _ := fakeEnv(t, "linux", newFakeHost())
	if got, err := linuxRunArgv(linEnv, linuxProviderNative, "", gate); err != nil ||
		strings.Join(got, " ") != strings.Join(gate, " ") {
		t.Errorf("native argv = %v, %v", got, err)
	}

	got, err = linuxRunArgv(linEnv, linuxProviderPodman, "localhost/bashy:1.0.0-linux-amd64", gate)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"/bin/bashy", "podman", "run", "--rm", "-i",
		"-v", "/home/liqiang/src/cloudbox:/w", "-w", "/w",
		"localhost/bashy:1.0.0-linux-amd64", "go", "test", "-short", "./..."}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("podman argv =\n%v\nwant\n%v", got, want)
	}

	if _, err := linuxRunArgv(linEnv, linuxProviderPodman, "", gate); err == nil {
		t.Error("the podman provider accepted an empty image")
	}
	if _, err := linuxRunArgv(linEnv, linuxProviderNative, "", nil); err == nil {
		t.Error("an empty command was accepted")
	}
}

// ---------------------------------------------------------------- arg parse

func TestParseLinuxArgs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		sub     string
		command []string
		wantErr bool
	}{
		{name: "no args is status", sub: "status"},
		{name: "probe json", args: []string{"probe", "--json"}, sub: "probe"},
		{name: "bare command", args: []string{"--", "go", "test"}, sub: "run", command: []string{"go", "test"}},
		{name: "explicit run", args: []string{"run", "--", "go", "test"}, sub: "run", command: []string{"go", "test"}},
		// Everything after -- belongs to the remote command, including words
		// that are bashy's own flags and subcommand names.
		{name: "flags after dash dash", args: []string{"--", "go", "test", "--json", "--host", "x"},
			sub: "run", command: []string{"go", "test", "--json", "--host", "x"}},
		{name: "subcommand name after dash dash", args: []string{"--", "probe"}, sub: "run", command: []string{"probe"}},
		{name: "host and command", args: []string{"--host", "liqiang@elephant.local:2222", "--", "go", "test"},
			sub: "run", command: []string{"go", "test"}},
		{name: "run without command", args: []string{"run"}, wantErr: true},
		{name: "command on a non-run subcommand", args: []string{"plan", "--", "go"}, wantErr: true},
		{name: "unknown subcommand", args: []string{"bootstrap"}, wantErr: true},
		{name: "unknown option", args: []string{"--nope"}, wantErr: true},
		{name: "unknown provider", args: []string{"--provider", "qemu"}, wantErr: true},
		{name: "empty command after dash dash", args: []string{"--"}, wantErr: true},
		{name: "two subcommands", args: []string{"plan", "probe"}, wantErr: true},
		{name: "flag without value", args: []string{"--provider"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLinuxArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parse(%v) = %+v, want an error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse(%v): %v", tc.args, err)
			}
			if got.sub != tc.sub {
				t.Errorf("sub = %q, want %q", got.sub, tc.sub)
			}
			if strings.Join(got.command, " ") != strings.Join(tc.command, " ") {
				t.Errorf("command = %v, want %v", got.command, tc.command)
			}
		})
	}
}

func TestParseLinuxHostSpec(t *testing.T) {
	for _, tc := range []struct {
		in, user, host, port string
		wantErr              bool
	}{
		{in: "elephant.local", host: "elephant.local"},
		{in: "liqiang@elephant.local:2222", user: "liqiang", host: "elephant.local", port: "2222"},
		{in: "liqiang@elephant.local", user: "liqiang", host: "elephant.local"},
		{in: "[fe80::1]:2222", host: "fe80::1", port: "2222"},
		{in: "", wantErr: true},
		{in: "@host", wantErr: true},
		{in: "host:", wantErr: true},
		{in: "a b", wantErr: true},
	} {
		got, err := parseLinuxHostSpec(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseLinuxHostSpec(%q) = %+v, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseLinuxHostSpec(%q): %v", tc.in, err)
			continue
		}
		if got.user != tc.user || got.host != tc.host || got.port != tc.port {
			t.Errorf("parseLinuxHostSpec(%q) = %+v, want %s/%s/%s", tc.in, got, tc.user, tc.host, tc.port)
		}
	}
}

// ---------------------------------------------------------------- host hop

// The acceptance command: ONE bashy invocation from Dragon that runs the
// cloudbox hub Linux gate on elephant.
func TestRunLinuxHostHopIsTheAcceptanceCommand(t *testing.T) {
	env, _, _ := fakeEnv(t, "darwin", newFakeHost())
	var got []string
	code := runLinux(context.Background(), env,
		[]string{"--host", "liqiang@elephant.local:2222", "--", "go", "test", "-short", "./..."},
		func(_ context.Context, _ linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := []string{"ssh", "-p", "2222", "liqiang@elephant.local", "bashy", "linux",
		"run", "--", "go", "test", "-short", "./..."}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("host argv =\n%v\nwant\n%v", got, want)
	}
}

func TestRunLinuxHostHopCarriesFlagsAndOmitsPortWhenUnset(t *testing.T) {
	env, _, _ := fakeEnv(t, "darwin", newFakeHost())
	var got []string
	code := runLinux(context.Background(), env,
		[]string{"--host", "elephant.local", "--json", "--provider", "wsl", "plan"},
		func(_ context.Context, _ linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := []string{"ssh", "elephant.local", "bashy", "linux", "--provider", "wsl", "--json", "plan"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("host argv =\n%v\nwant\n%v", got, want)
	}
}

// A --host hop must not probe the LOCAL host: the answer belongs to the remote.
func TestRunLinuxHostHopDoesNotProbeLocally(t *testing.T) {
	h := newFakeHost()
	env, _, _ := fakeEnv(t, "darwin", h)
	runLinux(context.Background(), env, []string{"--host", "elephant.local", "probe"},
		func(context.Context, linuxEnv, []string) int { return 0 })
	if len(h.calls) != 0 {
		t.Errorf("a --host hop probed the local host: %v", h.calls)
	}
}

// ---------------------------------------------------------------- CLI shape

// status is the scriptable readiness question, so it must fail on a host that
// cannot run Linux work — and still print its report.
func TestRunLinuxStatusExitCodes(t *testing.T) {
	h := elephantHost()
	env, out, _ := fakeEnv(t, "windows", h, "wsl.exe")
	if code := runLinux(context.Background(), env, []string{"status"}, nil); code != 1 {
		t.Errorf("status on an unready host exited %d, want 1", code)
	}
	if !strings.Contains(out.String(), "not ready via wsl") {
		t.Errorf("status report: %q", out.String())
	}

	h.reply("wsl.exe --status", utf16le("Default Version: 2\r\n"), 0, nil)
	h.reply("wsl.exe --list --quiet", utf16le("Ubuntu-24.04\r\n"), 0, nil)
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- sh -lc command -v go", []byte("/usr/local/bin/go\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, out, _ = fakeEnv(t, "windows", h, "wsl.exe")
	if code := runLinux(context.Background(), env, []string{"status"}, nil); code != 0 {
		t.Errorf("status on a converged host exited %d, want 0", code)
	}
	if !strings.Contains(out.String(), "ready via wsl") {
		t.Errorf("status report: %q", out.String())
	}
	// plan is a plan, never a verdict.
	env, _, _ = fakeEnv(t, "windows", elephantHost(), "wsl.exe")
	if code := runLinux(context.Background(), env, []string{"plan"}, nil); code != 0 {
		t.Errorf("plan on an unready host exited %d, want 0", code)
	}
}

func TestRunLinuxJSONOutput(t *testing.T) {
	h := elephantHost()
	env, out, _ := fakeEnv(t, "windows", h, "wsl.exe")
	if code := runLinux(context.Background(), env, []string{"plan", "--json"}, nil); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var plan struct {
		Provider string `json:"provider"`
		Ready    bool   `json:"ready"`
		Steps    []struct {
			ID         string   `json:"id"`
			Argv       []string `json:"argv"`
			Privileged bool     `json:"privileged"`
			Reboot     bool     `json:"reboot"`
			Done       bool     `json:"done"`
		} `json:"steps"`
		Facts struct {
			WSLInstalled bool `json:"wsl_installed"`
		} `json:"facts"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatalf("plan --json is not JSON: %v\n%s", err, out.String())
	}
	if plan.Provider != linuxProviderWSL || plan.Ready || plan.Facts.WSLInstalled {
		t.Errorf("plan = %+v", plan)
	}
	if len(plan.Steps) != 7 || plan.Steps[0].ID != "feature:VirtualMachinePlatform" ||
		!plan.Steps[0].Privileged || !plan.Steps[0].Reboot {
		t.Errorf("steps = %+v", plan.Steps)
	}
}

func TestRunLinuxRunRefusesOnAnUnreadyHost(t *testing.T) {
	h := elephantHost()
	env, _, errOut := fakeEnv(t, "windows", h, "wsl.exe")
	called := false
	code := runLinux(context.Background(), env, []string{"--", "go", "test"},
		func(context.Context, linuxEnv, []string) int { called = true; return 0 })
	if code == 0 || called {
		t.Fatalf("run proceeded on an unready host (exit %d, called=%t)", code, called)
	}
	if !strings.Contains(errOut.String(), "bashy linux up") {
		t.Errorf("refusal did not point at up:\n%s", errOut.String())
	}
}

func TestRunLinuxRunOnAConvergedHost(t *testing.T) {
	h := elephantHost()
	h.reply("wsl.exe --status", utf16le("Default Version: 2\r\n"), 0, nil)
	h.reply("wsl.exe --list --quiet", utf16le("Ubuntu-24.04\r\n"), 0, nil)
	h.reply("wsl.exe -d Ubuntu-24.04 -u root -- sh -lc command -v go", []byte("/usr/local/bin/go\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:VirtualMachinePlatform", []byte("State : Enabled\r\n"), 0, nil)
	h.reply("dism.exe /online /Get-FeatureInfo /FeatureName:Microsoft-Windows-Subsystem-Linux", []byte("State : Enabled\r\n"), 0, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")

	var got []string
	code := runLinux(context.Background(), env, []string{"--", "go", "test", "-short", "./..."},
		func(_ context.Context, _ linuxEnv, argv []string) int { got = argv; return 0 })
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "wsl.exe -d Ubuntu-24.04 -u root --cd /mnt/c/Users/liqia/src/cloudbox -- go test -short ./..."
	if strings.Join(got, " ") != want {
		t.Errorf("argv = %v\nwant %s", got, want)
	}
}

func TestRunLinuxHelpAndBadArgs(t *testing.T) {
	env, out, errOut := fakeEnv(t, "linux", newFakeHost())
	if code := runLinux(context.Background(), env, []string{"--help"}, nil); code != 0 {
		t.Fatalf("--help exited %d", code)
	}
	for _, want := range []string{"bashy linux", "probe", "plan", "up", "status", "run --", "--allow-reboot"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help missing %q:\n%s", want, out.String())
		}
	}
	if code := runLinux(context.Background(), env, []string{"--nope"}, nil); code != linuxRefusedStatus {
		t.Errorf("a bad option exited %d, want %d", code, linuxRefusedStatus)
	}
	if !strings.Contains(errOut.String(), "unknown option") {
		t.Errorf("bad option message: %q", errOut.String())
	}
}

func TestRunLinuxStepTreats3010AsSuccess(t *testing.T) {
	h := newFakeHost().reply("dism.exe", []byte("restart required"), 3010, nil)
	env, _, _ := fakeEnv(t, "windows", h, "wsl.exe")
	if err := runLinuxStep(context.Background(), env, linuxStep{Argv: []string{"dism.exe", "/online"}}); err != nil {
		t.Errorf("exit 3010 (restart required) reported as a failure: %v", err)
	}
	h = newFakeHost().reply("dism.exe", nil, 0, errors.New("boom"))
	env, _, _ = fakeEnv(t, "windows", h, "wsl.exe")
	if err := runLinuxStep(context.Background(), env, linuxStep{Argv: []string{"dism.exe"}}); err == nil {
		t.Error("a child that could not start was reported as success")
	}
}

func sliceHas(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}
