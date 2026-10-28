// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// `bashy linux` — stand up and reuse a Linux execution environment on ANY
// amd64 host, then run a command in it.
//
//	bashy linux probe  [--json]              what Linux providers this host has
//	bashy linux plan   [--json] [--provider P]  the ordered converge steps
//	bashy linux up     [--allow-privileged] [--allow-reboot]  converge (idempotent)
//	bashy linux status [--json]              one line: ready or not, and why
//	bashy linux [run] -- CMD [ARGS...]       run CMD in the Linux environment
//	bashy linux --host [user@]H[:port] ...   do any of the above on H
//
// WHY THIS VERB EXISTS. Linux-only work (the cloudbox hub gate needs
// linux/k3s, release image builds need podman) used to bottleneck on the one
// Windows host that happened to have WSL installed by hand. A host either has
// a Linux execution environment or it does not, and "install it by hand" does
// not converge a fleet. So the knowledge of how to get one lives here, ships
// in the release, and every host converges by running the same verb.
//
// PROVIDERS, in preference order. The first one that can work is chosen;
// --provider pins one.
//
//	native  GOOS=linux — the command runs here, nothing to stand up.
//	wsl     Windows — a bashy-managed WSL2 distro. The workspace needs no
//	        sync: a Windows path is already visible inside the distro under
//	        /mnt/<drive>, so `run` only translates the path.
//	podman  any OS — a Linux container through bashy's own podman, from
//	        bashy's own FROM-scratch image (`bashy self image`), so no
//	        third-party image enters the supply chain by default.
//
// PRIVILEGE IS NEVER TAKEN SILENTLY. Standing up WSL2 on a bare Windows host
// enables two Windows optional features and reboots the machine. On a host
// somebody else is using, that is not a step a tool may decide to take. So
// `plan` prints every step with its privilege and reboot flags, and `up`
// REFUSES a privileged or rebooting step unless --allow-privileged /
// --allow-reboot say so explicitly — printing the exact argv so a host's
// owner can authorize or run it themselves.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/binmgr"
)

const (
	linuxProviderNative = "native"
	linuxProviderWSL    = "wsl"
	linuxProviderPodman = "podman"
)

// linuxDistro is the ONE WSL distribution bashy manages. Pinning a single
// name is what makes two hosts' gate results comparable: a result only
// confirms a peer's result when both ran the same userland.
const linuxDistro = "Ubuntu-24.04"

// linuxWSLUser is the account `run` uses inside the distro. `wsl --install
// --no-launch` creates no interactive user, and a runner needs no login
// account, so bashy drives the distro as root rather than inventing one.
const linuxWSLUser = "root"

// linuxDistroBashy is where the toolchain step installs bashy's Linux build
// inside the distribution.
const linuxDistroBashy = "/usr/local/bin/bashy"

// linuxWindowsFeatures are the Windows optional features WSL2 needs. Order is
// the order they are enabled in.
var linuxWindowsFeatures = []string{"VirtualMachinePlatform", "Microsoft-Windows-Subsystem-Linux"}

// linuxRefusedStatus is `up`'s exit when a step needs authority this
// invocation was not given. It is not a failure: nothing was attempted.
const linuxRefusedStatus = 2

// linuxRun is the process seam. out is stdout+stderr combined, exit is the
// child status, err is non-nil only when the child could not be started.
type linuxRun func(ctx context.Context, name string, args ...string) (out []byte, exit int, err error)

// linuxEnv is everything `bashy linux` learns about the world through. Tests
// supply their own; nothing in the planner touches the host directly.
type linuxEnv struct {
	goos, goarch string
	self         string // this bashy executable
	cwd          string
	run          linuxRun
	look         func(string) (string, error)
	stdout       io.Writer
	stderr       io.Writer
}

// linuxFacts is the read-only capability report for one host.
type linuxFacts struct {
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	Provider  string   `json:"provider"`  // the one selected ("" = none usable)
	Providers []string `json:"providers"` // every provider that can work here

	WSLStub      bool     `json:"wsl_stub"`      // wsl.exe is on the host
	WSLInstalled bool     `json:"wsl_installed"` // the WSL component answers
	WSLDistros   []string `json:"wsl_distros"`
	DistroGo     bool     `json:"distro_go"` // a go is already on PATH in the distro

	WindowsEdition  string            `json:"windows_edition,omitempty"`
	WindowsFeatures map[string]string `json:"windows_features,omitempty"` // "Enabled"|"Disabled"|"" (unknown)
	Hypervisor      bool              `json:"hypervisor"`                 // a hypervisor is already running
	FirmwareVirt    string            `json:"firmware_virt,omitempty"`    // "yes"|"no"|"" (unknown)
	Elevated        bool              `json:"elevated"`

	Podman       bool   `json:"podman"` // bashy's podman answers
	Image        string `json:"image,omitempty"`
	ImagePresent bool   `json:"image_present"`
}

// linuxStep is one converge action. Argv is what it does, for printing; Run
// is set when the action is bashy's own work rather than one child process.
type linuxStep struct {
	ID         string   `json:"id"`
	Why        string   `json:"why"`
	Argv       []string `json:"argv"`
	Privileged bool     `json:"privileged"`
	// Reboot: authorizing this step authorizes restarting the host — either
	// because the step restarts it, or because its effect is only real after
	// a restart, which leaves the host half-changed until one happens.
	Reboot bool `json:"reboot"`
	// Restarts: this step restarts the host ITSELF, so nothing after it can
	// run this boot.
	Restarts bool `json:"restarts"`
	Done     bool `json:"done"`

	Run func(context.Context) error `json:"-"`
}

// linuxPlan is the whole converge answer for one host.
type linuxPlan struct {
	Provider string      `json:"provider"`
	Facts    linuxFacts  `json:"facts"`
	Steps    []linuxStep `json:"steps"`
	Ready    bool        `json:"ready"`             // nothing left to do
	Blocked  string      `json:"blocked,omitempty"` // a blocker no step can clear
}

// pending is the steps `up` would act on.
func (p linuxPlan) pending() []linuxStep {
	var out []linuxStep
	for _, s := range p.Steps {
		if !s.Done {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------- probe

// probeLinuxHost reports what this host can do. It is READ-ONLY: every command
// it runs only reports state, so it is safe on a host somebody else is using.
func probeLinuxHost(ctx context.Context, env linuxEnv) linuxFacts {
	f := linuxFacts{OS: env.goos, Arch: env.goarch}

	if env.goos == "linux" {
		f.Providers = append(f.Providers, linuxProviderNative)
	}
	if env.goos == "windows" {
		probeWindowsLinuxFacts(ctx, env, &f)
	}
	if _, exit, err := env.run(ctx, env.self, "podman", "info"); err == nil && exit == 0 {
		f.Podman = true
	}
	// podman can always be stood up where bashy ships its engine; on Windows
	// its own machine needs the same virtualization WSL does, so it is only
	// offered as an alternative once that much is true.
	if f.Podman || env.goos != "windows" {
		f.Providers = append(f.Providers, linuxProviderPodman)
	}
	f.Image = linuxPodmanImage(env)
	if f.Podman && f.Image != "" {
		if _, exit, err := env.run(ctx, env.self, "podman", "image", "exists", f.Image); err == nil && exit == 0 {
			f.ImagePresent = true
		}
	}
	return f
}

func probeWindowsLinuxFacts(ctx context.Context, env linuxEnv, f *linuxFacts) {
	if _, err := env.look("wsl.exe"); err == nil {
		f.WSLStub = true
	}
	if _, exit, err := env.run(ctx, "net.exe", "session"); err == nil && exit == 0 {
		f.Elevated = true
	}
	if out, exit, err := env.run(ctx, "reg.exe", "query",
		`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion`, "/v", "EditionID"); err == nil && exit == 0 {
		f.WindowsEdition = parseRegSZ(decodeConsole(out), "EditionID")
	}
	if f.WSLStub {
		if _, exit, err := env.run(ctx, "wsl.exe", "--status"); err == nil && exit == 0 {
			f.WSLInstalled = true
		}
		f.Providers = append(f.Providers, linuxProviderWSL)
	}
	if f.WSLInstalled {
		if out, exit, err := env.run(ctx, "wsl.exe", "--list", "--quiet"); err == nil && exit == 0 {
			f.WSLDistros = parseWSLDistros(decodeConsole(out))
		}
	}
	if slices.Contains(f.WSLDistros, linuxDistro) {
		if _, exit, err := env.run(ctx, "wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser,
			"--", "sh", "-lc", "command -v go"); err == nil && exit == 0 {
			f.DistroGo = true
		}
	}
	// dism /online needs administrator rights; without them the state stays
	// unknown and the plan keeps the (idempotent) enable step.
	if f.Elevated {
		f.WindowsFeatures = map[string]string{}
		for _, name := range linuxWindowsFeatures {
			out, exit, err := env.run(ctx, "dism.exe", "/online", "/Get-FeatureInfo", "/FeatureName:"+name)
			if err != nil || exit != 0 {
				f.WindowsFeatures[name] = ""
				continue
			}
			f.WindowsFeatures[name] = parseDISMState(decodeConsole(out))
		}
	}
	// systeminfo is the slow probe, so it only runs when the answer matters:
	// if the features are already on, whether the firmware allows them is moot.
	if !linuxFeaturesEnabled(*f) {
		if out, exit, err := env.run(ctx, "systeminfo.exe"); err == nil && exit == 0 {
			f.Hypervisor, f.FirmwareVirt = parseSystemInfoVirt(decodeConsole(out))
		}
	}
}

func linuxFeaturesEnabled(f linuxFacts) bool {
	for _, name := range linuxWindowsFeatures {
		if f.WindowsFeatures[name] != "Enabled" {
			return false
		}
	}
	return true
}

// linuxPodmanImage is the default container image: bashy's own, built from the
// release artifact by `bashy self image`, so the default path pulls nothing
// from a third-party registry.
func linuxPodmanImage(env linuxEnv) string {
	// A dev build carries no release version, so there is no artifact to name.
	// Say nothing rather than name a tag that cannot exist; --image then
	// decides.
	v := linuxImageVersion()
	if v == "" {
		return ""
	}
	return selfImageTag(v, linuxImageArch(env.goarch), nil)
}

// linuxImageVersion is the running bashy's version, which is the version of
// the Linux artifact that matches it. "" lets scratchArtifact resolve it.
func linuxImageVersion() string {
	return strings.TrimSpace(cli.BashyVersion())
}

func linuxImageArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// ---------------------------------------------------------------- parsers

// decodeConsole turns a Windows console tool's bytes into a Go string. wsl.exe
// writes UTF-16LE even when its output is redirected, which is why every
// parser here goes through one decoder instead of trusting string(out).
func decodeConsole(b []byte) string {
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		return decodeUTF16LE(b[2:])
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		return decodeUTF16BE(b[2:])
	}
	// No BOM: a UTF-16LE run of ASCII carries a NUL in (almost) every odd
	// byte. Two in three is far past anything UTF-8 text produces.
	pairs := len(b) / 2
	if pairs == 0 {
		return string(b)
	}
	nul := 0
	for i := 1; i < len(b); i += 2 {
		if b[i] == 0 {
			nul++
		}
	}
	if nul*3 >= pairs*2 {
		return decodeUTF16LE(b)
	}
	return string(b)
}

func decodeUTF16LE(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u))
}

func decodeUTF16BE(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}

// parseDISMState reads the State line of `dism /Get-FeatureInfo`.
func parseDISMState(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := cutFieldPrefix(line, "State")
		if !ok {
			continue
		}
		switch strings.ToLower(rest) {
		case "enabled", "enable pending":
			return "Enabled"
		case "disabled", "disable pending":
			return "Disabled"
		default:
			return rest
		}
	}
	return ""
}

// parseRegSZ reads one REG_SZ value out of `reg query` output.
func parseRegSZ(out, name string) string {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && fields[0] == name && strings.HasPrefix(fields[1], "REG_") {
			return strings.Join(fields[2:], " ")
		}
	}
	return ""
}

// parseWSLDistros reads `wsl --list --quiet`. An empty component list makes
// wsl print a sentence instead of a list, so anything with a space is not a
// distribution name.
func parseWSLDistros(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "(Default)"))
		if name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// parseSystemInfoVirt reads systeminfo's Hyper-V Requirements block. When a
// hypervisor is already running (VBS, Credential Guard, Hyper-V) systeminfo
// says so INSTEAD of listing the requirements — and that answer is the
// stronger one: the firmware must already allow virtualization.
func parseSystemInfoVirt(out string) (hypervisor bool, firmware string) {
	low := strings.ToLower(out)
	if strings.Contains(low, "a hypervisor has been detected") {
		return true, "yes"
	}
	for _, line := range strings.Split(out, "\n") {
		rest, ok := cutFieldPrefix(strings.TrimSpace(line), "Virtualization Enabled In Firmware")
		if !ok {
			continue
		}
		switch strings.ToLower(rest) {
		case "yes":
			return false, "yes"
		case "no":
			return false, "no"
		}
	}
	return false, ""
}

// cutFieldPrefix matches "<name><spaces><sep><spaces><value>" where sep is ':'
// — the shape dism, systeminfo and reg all print.
func cutFieldPrefix(line, name string) (string, bool) {
	if len(line) < len(name) || !strings.EqualFold(line[:len(name)], name) {
		return "", false
	}
	rest := strings.TrimSpace(line[len(name):])
	if !strings.HasPrefix(rest, ":") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, ":")), true
}

// ---------------------------------------------------------------- provider

// selectLinuxProvider picks the provider to use. want pins one and is an error
// when this host cannot offer it, so a pinned run never silently uses another.
func selectLinuxProvider(f linuxFacts, want string) (string, error) {
	if want != "" {
		if !slices.Contains(f.Providers, want) {
			return "", fmt.Errorf("provider %q is not available on %s/%s (available: %s)",
				want, f.OS, f.Arch, strings.Join(f.Providers, ", "))
		}
		return want, nil
	}
	for _, p := range []string{linuxProviderNative, linuxProviderWSL, linuxProviderPodman} {
		if slices.Contains(f.Providers, p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Linux execution environment is available or installable on %s/%s", f.OS, f.Arch)
}

// ---------------------------------------------------------------- plan

// planLinuxRunner is the whole decision: given facts, what has to happen
// before a Linux command can run here. Every step is idempotent, so a plan
// against a converged host has no pending steps and `up` is a no-op.
func planLinuxRunner(env linuxEnv, f linuxFacts, want string) linuxPlan {
	provider, err := selectLinuxProvider(f, want)
	if err != nil {
		return linuxPlan{Facts: f, Blocked: err.Error()}
	}
	f.Provider = provider
	p := linuxPlan{Provider: provider, Facts: f}
	switch provider {
	case linuxProviderNative:
		// Nothing to stand up: this host IS the Linux environment.
	case linuxProviderWSL:
		p = planWSL(env, f)
	case linuxProviderPodman:
		p = planPodman(env, f)
	}
	p.Ready = p.Blocked == "" && len(p.pending()) == 0
	return p
}

func planWSL(env linuxEnv, f linuxFacts) linuxPlan {
	p := linuxPlan{Provider: linuxProviderWSL, Facts: f}
	if !f.WSLStub {
		p.Blocked = "this Windows build ships no wsl.exe (WSL2 needs Windows 10 2004+ / 11 / Server 2022+); use --provider podman or a newer host"
		return p
	}
	if !f.Hypervisor && f.FirmwareVirt == "no" {
		p.Blocked = "hardware virtualization is disabled in this host's firmware; no software step can enable it (a BIOS/UEFI change is needed at the machine)"
		return p
	}

	featuresEnabled := linuxFeaturesEnabled(f)
	for _, name := range linuxWindowsFeatures {
		p.Steps = append(p.Steps, linuxStep{
			ID:  "feature:" + name,
			Why: "WSL2 needs the " + name + " Windows optional feature",
			Argv: []string{"dism.exe", "/online", "/Enable-Feature",
				"/FeatureName:" + name, "/All", "/NoRestart"},
			Privileged: true,
			Reboot:     true,
			// Already installed WSL proves the features are on even when dism
			// could not be read (an unelevated probe).
			Done: f.WSLInstalled || f.WindowsFeatures[name] == "Enabled",
		})
	}
	p.Steps = append(p.Steps, linuxStep{
		ID:         "restart",
		Why:        "the Windows optional features above only take effect after a restart",
		Argv:       []string{"shutdown.exe", "/r", "/t", "60", "/c", "bashy linux: enabling WSL2"},
		Privileged: true,
		Reboot:     true,
		Restarts:   true,
		Done:       f.WSLInstalled || featuresEnabled,
	})
	// The component is installed from Microsoft's own MSI, not by wsl.exe.
	// Until the component exists, the wsl.exe in System32 is only the Windows
	// inbox STUB, whose entire argument list is --install/--update/--status/
	// --help — and on elephant (Windows 11 26300, 2026-10-28) EVERY install
	// route through it, `--update`, `--install`, `--install --no-launch` and
	// `--install --no-distribution` alike, answered the same
	//
	//	The Windows Subsystem for Linux is not installed. You can install by
	//	running 'wsl.exe --install'.
	//
	// and exited 1. The stub's own install path wants the Store, which an
	// unattended session on a server-shaped host does not have, so it cannot
	// bootstrap itself and a fleet cannot converge through it.
	//
	// Microsoft publishes the component as a plain MSI for exactly this
	// offline/enterprise case, and `msiexec /quiet` installs it with no Store
	// and no console. That is a runtime download + exec of a separate program
	// (WSL is MIT-licensed), so no license propagates — the same standing as
	// podman or ollama under docs/licensing-supply-chain-policy.md.
	p.Steps = append(p.Steps, linuxStep{
		ID:         "wsl-component",
		Why:        "install the WSL component from Microsoft's MSI (until it exists, wsl.exe is only the Windows inbox stub and cannot install itself)",
		Argv:       []string{"msiexec.exe", "/i", wslMSIName(env.goarch), "/quiet", "/norestart"},
		Privileged: true,
		Done:       f.WSLInstalled,
		Run:        func(ctx context.Context) error { return installWSLComponent(ctx, env) },
	})
	p.Steps = append(p.Steps, linuxStep{
		ID:   "wsl-default-version",
		Why:  "a WSL1 distribution cannot run the Linux kernel work this verb exists for",
		Argv: []string{"wsl.exe", "--set-default-version", "2"},
		Done: len(f.WSLDistros) > 0,
	})
	p.Steps = append(p.Steps, linuxStep{
		ID:   "distro",
		Why:  "install the one distribution bashy manages (" + linuxDistro + ")",
		Argv: []string{"wsl.exe", "--install", "--no-launch", "-d", linuxDistro},
		Done: slices.Contains(f.WSLDistros, linuxDistro),
	})
	p.Steps = append(p.Steps, linuxStep{
		ID:   "toolchain",
		Why:  "put bashy and its managed Go inside the distribution",
		Argv: []string{"wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser, "--", "bashy", "go", "version"},
		Done: f.DistroGo,
		Run:  func(ctx context.Context) error { return provisionWSLToolchain(ctx, env) },
	})
	return p
}

func planPodman(env linuxEnv, f linuxFacts) linuxPlan {
	p := linuxPlan{Provider: linuxProviderPodman, Facts: f}
	p.Steps = append(p.Steps, linuxStep{
		ID:   "podman-machine",
		Why:  "bashy's podman needs its Linux VM running on a non-Linux host",
		Argv: []string{env.self, "podman", "machine", "start"},
		Done: f.Podman || env.goos == "linux",
	})
	if f.Image == "" {
		p.Blocked = "this bashy build carries no release version, so its Linux image cannot be named: pass --image NAME@sha256:<digest>, or run a released bashy"
		return p
	}
	p.Steps = append(p.Steps, linuxStep{
		ID:   "image",
		Why:  "build bashy's own FROM-scratch Linux image " + f.Image + " (no third-party image is pulled)",
		Argv: []string{env.self, "self", "image"},
		Done: f.ImagePresent,
	})
	return p
}

// wslMSIRepo is where Microsoft publishes the WSL component. The releases
// there carry a per-asset sha256 in the GitHub API, so binmgr verifies the
// download the same way `bashy self image` verifies bashy's own artifact.
const wslMSIRepo = "microsoft/WSL"

// wslMSIName is the asset for one architecture: wsl.<version>.x64.msi. The
// version sits in the middle of the name, so the match is on the tail.
func wslMSIName(goarch string) string {
	return "wsl.<version>." + wslMSIArch(goarch) + ".msi"
}

func wslMSIArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "x64"
}

// wslMSIAssetMatch picks the MSI for this architecture out of a release that
// also ships an msixbundle and a nupkg. x64 and arm64 both end in ".msi", so
// the architecture token has to be part of the suffix or an arm64 host would
// take the x64 package.
func wslMSIAssetMatch(goarch string) func(name, goos, hostArch string) bool {
	suffix := "." + wslMSIArch(goarch) + ".msi"
	return func(name, _, _ string) bool {
		return strings.HasSuffix(strings.ToLower(name), suffix)
	}
}

// installWSLComponent downloads Microsoft's WSL MSI and installs it quietly.
// /norestart is deliberate: the optional features already took their restart,
// and the MSI does not need another — a step that rebooted here would be
// outside what `up` was authorized to do.
func installWSLComponent(ctx context.Context, env linuxEnv) error {
	tool, err := binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
		Name:       "wsl-msi-" + wslMSIArch(env.goarch),
		Repo:       wslMSIRepo,
		AssetMatch: wslMSIAssetMatch(env.goarch),
	})
	if err != nil {
		return fmt.Errorf("resolve the WSL component MSI: %w", err)
	}
	msi, err := binmgr.Ensure(ctx, tool)
	if err != nil {
		return fmt.Errorf("fetch the WSL component MSI: %w", err)
	}
	fmt.Fprintf(env.stderr, "bashy linux: WSL component %s@%s\n", wslMSIRepo, tool.Version)
	out, exit, err := env.run(ctx, "msiexec.exe", "/i", msi, "/quiet", "/norestart")
	if err != nil {
		return err
	}
	// 3010 is "succeeded, restart required": the component is in, and the
	// steps after this one go through wsl.exe, which starts its own services.
	if exit != 0 && exit != 3010 {
		return fmt.Errorf("msiexec /i %s exited %d: %s", msi, exit, strings.TrimSpace(decodeConsole(out)))
	}
	return nil
}

// provisionWSLToolchain puts bashy's Linux build inside the distribution and
// lets it provision its own managed Go. The artifact is the checksum-verified
// release scratch binary — the same one `bashy self image` uses — so a Windows
// host can provision a Linux userland without a Linux host to copy from.
func provisionWSLToolchain(ctx context.Context, env linuxEnv) error {
	artifact, _, err := scratchArtifact(ctx, env.stderr, linuxImageVersion(), linuxImageArch(env.goarch))
	if err != nil {
		return fmt.Errorf("fetch the linux bashy artifact: %w", err)
	}
	inside, err := wslPath(artifact)
	if err != nil {
		return err
	}
	for _, argv := range [][]string{
		{"wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser, "--", "install", "-m", "0755", inside, linuxDistroBashy},
		{"wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser, "--", linuxDistroBashy, "go", "version"},
	} {
		out, exit, err := env.run(ctx, argv[0], argv[1:]...)
		if err != nil {
			return err
		}
		if exit != 0 {
			return fmt.Errorf("%s exited %d: %s", strings.Join(argv, " "), exit, strings.TrimSpace(decodeConsole(out)))
		}
	}
	return linkDistroGo(ctx, env)
}

// linkDistroGo puts the managed toolchain's go on PATH inside the distro.
//
// Two reasons it is not optional. A gate is written `go test ./...` — the
// command a peer ran, and the command this verb exists to reproduce — so a go
// reachable only as `bashy go` would make every caller rewrite its gate.
// And `command -v go` is the toolchain step's own Done predicate: without the
// link the step never converges, `up` re-provisions on every run, and the
// no-op property the whole plan is built on is lost (measured on elephant
// 2026-10-28: `up` reported "wsl is set up", then `status` still said
// "1 step(s) left: toolchain").
func linkDistroGo(ctx context.Context, env linuxEnv) error {
	out, exit, err := env.run(ctx, "wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser,
		"--", linuxDistroBashy, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	goroot := strings.TrimSpace(decodeConsole(out))
	if exit != 0 || goroot == "" {
		return fmt.Errorf("ask the distro's bashy for GOROOT: exited %d: %s", exit, goroot)
	}
	out, exit, err = env.run(ctx, "wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser,
		"--", "ln", "-sfn", goroot+"/bin/go", "/usr/local/bin/go")
	if err != nil {
		return err
	}
	if exit != 0 {
		return fmt.Errorf("link %s/bin/go onto PATH: exited %d: %s", goroot, exit, strings.TrimSpace(decodeConsole(out)))
	}
	return nil
}

// ---------------------------------------------------------------- run

// wslPath translates a Windows path to the path the same bytes have inside a
// WSL distribution. The workspace needs no sync because of this: the files are
// already there.
func wslPath(win string) (string, error) {
	p := strings.ReplaceAll(win, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		drive := strings.ToLower(p[:1])
		rest := strings.TrimPrefix(p[2:], "/")
		if rest == "" {
			return "/mnt/" + drive, nil
		}
		return "/mnt/" + drive + "/" + rest, nil
	}
	if strings.HasPrefix(p, "/") {
		return p, nil
	}
	return "", fmt.Errorf("cannot translate %q into a path inside %s: expected a drive-letter or absolute path", win, linuxDistro)
}

// linuxRunArgv is the exact child process `run` starts for one provider.
func linuxRunArgv(env linuxEnv, provider, image string, command []string) ([]string, error) {
	if len(command) == 0 {
		return nil, errors.New("expected a command after --")
	}
	switch provider {
	case linuxProviderNative:
		return command, nil
	case linuxProviderWSL:
		dir, err := wslPath(env.cwd)
		if err != nil {
			return nil, err
		}
		argv := []string{"wsl.exe", "-d", linuxDistro, "-u", linuxWSLUser, "--cd", dir, "--"}
		return append(argv, command...), nil
	case linuxProviderPodman:
		if image == "" {
			return nil, errors.New("no container image: pass --image NAME@sha256:<digest> (a dev build cannot name its own release image)")
		}
		argv := []string{env.self, "podman", "run", "--rm", "-i",
			"-v", env.cwd + ":/w", "-w", "/w", image}
		return append(argv, command...), nil
	}
	return nil, fmt.Errorf("unknown provider %q", provider)
}

// ---------------------------------------------------------------- host hop

// linuxHostSpec is [user@]host[:port], the shape operators already write for
// a peer host.
type linuxHostSpec struct {
	user, host, port string
}

func parseLinuxHostSpec(spec string) (linuxHostSpec, error) {
	var h linuxHostSpec
	s := strings.TrimSpace(spec)
	if s == "" || strings.ContainsAny(s, " \t\x00\r\n") {
		return h, fmt.Errorf("invalid host %q", spec)
	}
	if user, rest, ok := strings.Cut(s, "@"); ok {
		if user == "" || rest == "" {
			return h, fmt.Errorf("invalid host %q", spec)
		}
		h.user, s = user, rest
	}
	// An IPv6 literal must be bracketed for the port split to be unambiguous.
	if strings.HasPrefix(s, "[") {
		addr, rest, ok := strings.Cut(strings.TrimPrefix(s, "["), "]")
		if !ok || addr == "" {
			return h, fmt.Errorf("invalid host %q", spec)
		}
		h.host = addr
		s = rest
	} else if host, port, ok := strings.Cut(s, ":"); ok {
		h.host, s = host, ":"+port
	} else {
		h.host, s = s, ""
	}
	if port := strings.TrimPrefix(s, ":"); s != "" {
		if port == "" || strings.ContainsAny(port, ":") {
			return h, fmt.Errorf("invalid port in host %q", spec)
		}
		h.port = port
	}
	if h.host == "" {
		return h, fmt.Errorf("invalid host %q", spec)
	}
	return h, nil
}

// linuxSSHArgv is `ssh [-p PORT] [user@]host` — the prefix every hop shares.
func linuxSSHArgv(spec linuxHostSpec) []string {
	argv := []string{"ssh"}
	if spec.port != "" {
		argv = append(argv, "-p", spec.port)
	}
	target := spec.host
	if spec.user != "" {
		target = spec.user + "@" + spec.host
	}
	return append(argv, target)
}

// linuxRemoteBashy is how to run bashy on the host. It is a command, not just
// a path, and is split into words the way BASHY_OCI already is — so a host
// whose bashy is not on PATH, or one that needs an `env VAR=V` prefix for a
// bootstrap, is reachable without a second flag for each case.
func linuxRemoteBashy(remote string) []string {
	if f := strings.Fields(remote); len(f) > 0 {
		return f
	}
	return []string{"bashy"}
}

// linuxHostArgv re-issues this very invocation on another host over the host
// OS ssh transport `bashy remote` already bootstraps with — the same way peers
// run jobs today. --host is stripped so the remote side runs locally.
func linuxHostArgv(spec linuxHostSpec, remote string, args []string) []string {
	remoteArgv := append(linuxRemoteBashy(remote), "linux")
	remoteArgv = append(remoteArgv, args...)
	return append(linuxSSHArgv(spec), shellJoin(remoteArgv))
}

// shellJoin renders an argv as ONE word-for-word command line for a remote
// shell. ssh offers no argv: it concatenates its trailing arguments and the
// remote LOGIN SHELL re-parses the result, so an unquoted hop silently loses
// a command's own quoting — `run -- sh -lc 'a; b'` arrived as `b` running on
// the host instead of inside the Linux environment (measured on elephant
// 2026-10-28). Quoting here is what makes the remote command the same
// command.
func shellJoin(argv []string) string {
	words := make([]string, 0, len(argv))
	for _, a := range argv {
		// A word that cannot be reinterpreted is left bare, so the hop a
		// refusal prints stays the argv an operator can paste back.
		if a != "" && !strings.ContainsFunc(a, needsShellQuote) {
			words = append(words, a)
			continue
		}
		words = append(words, shellQuote(a))
	}
	return strings.Join(words, " ")
}

func needsShellQuote(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("_@%+=:,./-", r)
}

// ---------------------------------------------------------------- CLI

const linuxHelp = `Usage: bashy linux [--host [user@]HOST[:PORT]] <command>

Stand up and reuse a Linux execution environment on this host, then run a
command in it. Providers, in preference order: native (GOOS=linux), wsl
(a bashy-managed WSL2 ` + linuxDistro + `), podman (bashy's own Linux image).
The workspace is not copied: the current directory is the working directory
inside the Linux environment.

Commands:
  probe              report what Linux providers this host has (read-only)
  plan               the ordered, idempotent steps that would converge it
  up                 run those steps (a converged host is a no-op)
  status             one line: ready or not, and why
  run -- CMD [ARG…]  run CMD in the Linux environment (also the bare form)

Examples:
  bashy linux probe --json
  bashy linux -- go test -short ./...
  bashy linux --host liqiang@elephant.local:2222 plan
  bashy linux --host liqiang@elephant.local:2222 -- go test -short ./...
  bashy linux --host elephant.local --sync -- go test -short ./...

Options:
  --host SPEC          do all of this on another host, over ssh
  --remote-bashy CMD   how to run bashy on --host (default: bashy; split into
                       words, so "env VAR=V /path/bashy" works)
  --sync               copy this workspace to --host first and run in it
  --dir PATH           the directory to run in (default: the current one)
  --provider P         pin a provider (native|wsl|podman)
  --image REF          podman provider image (third-party refs must be digests)
  --json               machine-readable output for probe/plan/status
  --allow-privileged   let 'up' run a step that needs administrator rights
  --allow-reboot       let 'up' run a step that restarts the host
  -h, --help           show this help
`

type linuxOptions struct {
	host            string
	remoteBashy     string
	dir             string
	sync            bool
	provider        string
	image           string
	asJSON          bool
	allowPrivileged bool
	allowReboot     bool
	help            bool
	command         []string
	sub             string
	rest            []string
}

var linuxSubcommands = []string{"probe", "plan", "up", "status", "run"}

// parseLinuxArgs stops interpreting options at --, so a remote command's own
// flags never become bashy's.
func parseLinuxArgs(args []string) (linuxOptions, error) {
	var o linuxOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := func(name string) (string, error) {
			if v, ok := strings.CutPrefix(arg, name+"="); ok {
				if v == "" {
					return "", fmt.Errorf("%s needs a value", name)
				}
				return v, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		switch {
		case arg == "--":
			o.command = append([]string(nil), args[i+1:]...)
			if len(o.command) == 0 {
				return o, errors.New("expected a command after --")
			}
			if o.sub == "" {
				o.sub = "run"
			}
			if o.sub != "run" {
				return o, fmt.Errorf("%s takes no command after --", o.sub)
			}
			// -- ends the options, so the checks below the loop are made here
			// too: an option is not excused from them by a command following.
			return o, o.validate()
		case arg == "-h" || arg == "--help":
			o.help = true
		case arg == "--json":
			o.asJSON = true
		case arg == "--allow-privileged":
			o.allowPrivileged = true
		case arg == "--allow-reboot":
			o.allowReboot = true
		case arg == "--sync":
			o.sync = true
		case arg == "--host" || strings.HasPrefix(arg, "--host="):
			v, err := value("--host")
			if err != nil {
				return o, err
			}
			o.host = v
		case arg == "--remote-bashy" || strings.HasPrefix(arg, "--remote-bashy="):
			v, err := value("--remote-bashy")
			if err != nil {
				return o, err
			}
			o.remoteBashy = v
		case arg == "--dir" || strings.HasPrefix(arg, "--dir="):
			v, err := value("--dir")
			if err != nil {
				return o, err
			}
			o.dir = v
		case arg == "--provider" || strings.HasPrefix(arg, "--provider="):
			v, err := value("--provider")
			if err != nil {
				return o, err
			}
			if !slices.Contains([]string{linuxProviderNative, linuxProviderWSL, linuxProviderPodman}, v) {
				return o, fmt.Errorf("unknown provider %q (native|wsl|podman)", v)
			}
			o.provider = v
		case arg == "--image" || strings.HasPrefix(arg, "--image="):
			v, err := value("--image")
			if err != nil {
				return o, err
			}
			o.image = v
		case strings.HasPrefix(arg, "-"):
			return o, fmt.Errorf("unknown option %q", arg)
		default:
			if o.sub != "" {
				return o, fmt.Errorf("unexpected argument %q (put a command after --)", arg)
			}
			if !slices.Contains(linuxSubcommands, arg) {
				return o, fmt.Errorf("unknown command %q (%s)", arg, strings.Join(linuxSubcommands, "|"))
			}
			o.sub = arg
		}
	}
	if o.sub == "" && !o.help {
		o.sub = "status"
	}
	if o.sub == "run" && len(o.command) == 0 && !o.help {
		return o, errors.New("run needs a command: bashy linux run -- CMD [ARGS...]")
	}
	return o, o.validate()
}

// validate holds the checks that are about the options as a WHOLE, so both
// ways out of the parse loop apply them.
func (o linuxOptions) validate() error {
	if o.help {
		return nil
	}
	if o.host == "" {
		if o.sync {
			return errors.New("--sync needs --host: there is no other host to copy the workspace to")
		}
		if o.remoteBashy != "" {
			return errors.New("--remote-bashy needs --host: it says how to run bashy on the OTHER host")
		}
	}
	if o.sync && o.dir != "" {
		return errors.New("--sync chooses the directory on the host, so it cannot be combined with --dir")
	}
	return nil
}

// hostArgs rebuilds this invocation without --host, for the remote side.
func (o linuxOptions) hostArgs() []string {
	var args []string
	if o.dir != "" {
		args = append(args, "--dir", o.dir)
	}
	if o.provider != "" {
		args = append(args, "--provider", o.provider)
	}
	if o.image != "" {
		args = append(args, "--image", o.image)
	}
	if o.asJSON {
		args = append(args, "--json")
	}
	if o.allowPrivileged {
		args = append(args, "--allow-privileged")
	}
	if o.allowReboot {
		args = append(args, "--allow-reboot")
	}
	args = append(args, o.sub)
	if len(o.command) > 0 {
		args = append(args, "--")
		args = append(args, o.command...)
	}
	return args
}

func dispatchLinux(args []string) int {
	env := linuxEnv{
		goos:   runtime.GOOS,
		goarch: runtime.GOARCH,
		self:   selfExecutable(),
		run:    hostLinuxRun,
		look:   exec.LookPath,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	if cwd, err := os.Getwd(); err == nil {
		env.cwd = cwd
	}
	return runLinux(context.Background(), env, args, execLinuxArgv)
}

// hostLinuxRun is the real probe/step runner: combined output, child status.
func hostLinuxRun(ctx context.Context, name string, args ...string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out, exit.ExitCode(), nil
	}
	return out, -1, err
}

// execLinuxArgv runs the chosen command line with this process's streams, so
// a test's output is the operator's output.
func execLinuxArgv(ctx context.Context, env linuxEnv, argv []string) int {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = env.cwd
	cmd.Stdin = os.Stdin
	cmd.Stdout = env.stdout
	cmd.Stderr = env.stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		fmt.Fprintln(env.stderr, "bashy linux:", err)
		return 1
	}
	return 0
}

func runLinux(ctx context.Context, env linuxEnv, args []string, execArgv func(context.Context, linuxEnv, []string) int) int {
	opts, err := parseLinuxArgs(args)
	if err != nil {
		fmt.Fprintln(env.stderr, "bashy linux:", err)
		return linuxRefusedStatus
	}
	if opts.help {
		fmt.Fprint(env.stdout, linuxHelp)
		return 0
	}
	if opts.host != "" {
		spec, err := parseLinuxHostSpec(opts.host)
		if err != nil {
			fmt.Fprintln(env.stderr, "bashy linux:", err)
			return linuxRefusedStatus
		}
		if opts.sync {
			dir, err := syncLinuxWorkspace(ctx, env, spec, opts.remoteBashy)
			if err != nil {
				fmt.Fprintln(env.stderr, "bashy linux: sync:", err)
				return 1
			}
			opts.dir = dir
		}
		return execArgv(ctx, env, linuxHostArgv(spec, opts.remoteBashy, opts.hostArgs()))
	}
	if opts.dir != "" {
		// --dir is the workspace to run in. It replaces the cwd outright, so
		// every provider (the /mnt translation, the bind mount, the child's
		// own directory) sees one answer.
		env.cwd = normalizeHostDir(env.goos, opts.dir)
	}

	facts := probeLinuxHost(ctx, env)
	if opts.sub == "probe" {
		if p, err := selectLinuxProvider(facts, opts.provider); err == nil {
			facts.Provider = p
		}
		return emitLinux(env, opts.asJSON, facts, func() {
			printLinuxFacts(env.stdout, facts)
		})
	}

	plan := planLinuxRunner(env, facts, opts.provider)
	switch opts.sub {
	case "plan":
		return emitLinux(env, opts.asJSON, plan, func() { printLinuxPlan(env.stdout, plan) })
	case "status":
		// status is the scriptable question "can I run Linux work here?", so
		// a not-ready host is a non-zero answer even though the report itself
		// succeeded. plan stays 0: a plan is never a verdict.
		if code := emitLinux(env, opts.asJSON, plan, func() { printLinuxStatus(env.stdout, plan) }); code != 0 {
			return code
		}
		if !plan.Ready {
			return 1
		}
		return 0
	case "up":
		return linuxUp(ctx, env, plan, opts)
	case "run":
		if !plan.Ready {
			fmt.Fprintf(env.stderr, "bashy linux: the %s provider is not ready here; run `bashy linux up` first:\n", plan.Provider)
			printLinuxPlan(env.stderr, plan)
			return 1
		}
		image := opts.image
		if image == "" {
			image = facts.Image
		}
		argv, err := linuxRunArgv(env, plan.Provider, image, opts.command)
		if err != nil {
			fmt.Fprintln(env.stderr, "bashy linux:", err)
			return linuxRefusedStatus
		}
		return execArgv(ctx, env, argv)
	}
	fmt.Fprintf(env.stderr, "bashy linux: unknown command %q\n", opts.sub)
	return linuxRefusedStatus
}

func emitLinux(env linuxEnv, asJSON bool, value any, text func()) int {
	if !asJSON {
		text()
		return 0
	}
	enc := json.NewEncoder(env.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Fprintln(env.stderr, "bashy linux:", err)
		return 1
	}
	return 0
}

func linuxUp(ctx context.Context, env linuxEnv, plan linuxPlan, opts linuxOptions) int {
	if plan.Blocked != "" {
		fmt.Fprintln(env.stderr, "bashy linux: blocked:", plan.Blocked)
		return 1
	}
	pending := plan.pending()
	if len(pending) == 0 {
		fmt.Fprintf(env.stdout, "bashy linux: %s is already set up here; nothing to do\n", plan.Provider)
		return 0
	}
	for _, step := range pending {
		switch {
		case step.Privileged && !opts.allowPrivileged:
			return refuseLinuxStep(env, step, "needs administrator rights", "--allow-privileged")
		case step.Reboot && !opts.allowReboot:
			return refuseLinuxStep(env, step, "restarts this host", "--allow-reboot")
		}
		fmt.Fprintf(env.stdout, "bashy linux: %s — %s\n  %s\n", step.ID, step.Why, strings.Join(step.Argv, " "))
		if err := runLinuxStep(ctx, env, step); err != nil {
			fmt.Fprintf(env.stderr, "bashy linux: step %s failed: %v\n", step.ID, err)
			return 1
		}
		if step.Restarts {
			fmt.Fprintln(env.stdout, "bashy linux: this host is restarting; re-run `bashy linux up` afterwards to finish")
			return 0
		}
	}
	fmt.Fprintf(env.stdout, "bashy linux: %s is set up\n", plan.Provider)
	return 0
}

func runLinuxStep(ctx context.Context, env linuxEnv, step linuxStep) error {
	if step.Run != nil {
		return step.Run(ctx)
	}
	out, exit, err := env.run(ctx, step.Argv[0], step.Argv[1:]...)
	if err != nil {
		return err
	}
	// 3010 is Windows' "succeeded, restart required".
	if exit != 0 && exit != 3010 {
		return fmt.Errorf("exited %d: %s", exit, strings.TrimSpace(decodeConsole(out)))
	}
	return nil
}

// refuseLinuxStep stops without touching the host and prints the exact step,
// so whoever owns the machine can authorize it or run it themselves.
func refuseLinuxStep(env linuxEnv, step linuxStep, what, flag string) int {
	fmt.Fprintf(env.stderr, `bashy linux: refusing to continue: step %q %s.
  why:  %s
  step: %s
Nothing was changed. Re-run with %s once the host's owner has agreed.
`, step.ID, what, step.Why, strings.Join(step.Argv, " "), flag)
	return linuxRefusedStatus
}

func printLinuxFacts(w io.Writer, f linuxFacts) {
	fmt.Fprintf(w, "host       %s/%s\n", f.OS, f.Arch)
	fmt.Fprintf(w, "providers  %s\n", strings.Join(orNone(f.Providers), ", "))
	if f.Provider != "" {
		fmt.Fprintf(w, "selected   %s\n", f.Provider)
	}
	if f.OS == "windows" {
		fmt.Fprintf(w, "windows    edition=%s elevated=%t hypervisor=%t firmware-virt=%s\n",
			orUnknown(f.WindowsEdition), f.Elevated, f.Hypervisor, orUnknown(f.FirmwareVirt))
		fmt.Fprintf(w, "wsl        stub=%t installed=%t distros=%s go-in-distro=%t\n",
			f.WSLStub, f.WSLInstalled, strings.Join(orNone(f.WSLDistros), ","), f.DistroGo)
		for _, name := range linuxWindowsFeatures {
			fmt.Fprintf(w, "feature    %s=%s\n", name, orUnknown(f.WindowsFeatures[name]))
		}
	}
	fmt.Fprintf(w, "podman     reachable=%t image=%s present=%t\n", f.Podman, orUnknown(f.Image), f.ImagePresent)
}

func printLinuxPlan(w io.Writer, p linuxPlan) {
	if p.Blocked != "" {
		fmt.Fprintln(w, "blocked:", p.Blocked)
		return
	}
	fmt.Fprintf(w, "provider %s — %s\n", p.Provider, readyWord(p.Ready))
	for _, s := range p.Steps {
		mark, tags := "todo", ""
		if s.Done {
			mark = "done"
		}
		if s.Privileged {
			tags += " [admin]"
		}
		if s.Reboot {
			tags += " [reboot]"
		}
		fmt.Fprintf(w, "  %s  %s%s\n        %s\n        %s\n", mark, s.ID, tags, s.Why, strings.Join(s.Argv, " "))
	}
}

func printLinuxStatus(w io.Writer, p linuxPlan) {
	if p.Blocked != "" {
		fmt.Fprintln(w, "linux: blocked —", p.Blocked)
		return
	}
	if p.Ready {
		fmt.Fprintf(w, "linux: ready via %s\n", p.Provider)
		return
	}
	var ids []string
	for _, s := range p.pending() {
		ids = append(ids, s.ID)
	}
	fmt.Fprintf(w, "linux: not ready via %s — %d step(s) left: %s (run `bashy linux up`)\n",
		p.Provider, len(ids), strings.Join(ids, ", "))
}

func readyWord(ready bool) string {
	if ready {
		return "ready"
	}
	return "not ready"
}

func orNone(items []string) []string {
	if len(items) == 0 {
		return []string{"none"}
	}
	return items
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}

// selfExecutable is this bashy, for the steps and runs that re-enter it.
func selfExecutable() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "bashy"
}
