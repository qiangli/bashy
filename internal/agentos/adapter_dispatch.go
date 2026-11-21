// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/pkg/shape/slot"
	"github.com/qiangli/yoke/pkg/src"
)

const (
	adapterExecEnv   = "BASHY_ADAPTER_EXEC"
	adapterInprocEnv = "BASHY_ADAPTER_INPROC"
	adaptersRootEnv  = "BASHY_ADAPTERS_ROOT"
	invokedAsEnv     = "BASHY_INVOKED_AS"
)

// adaptedVerbs maps front-door verb spellings (including aliases) to their
// folder and entry directory under the bashy repository.
var adaptedVerbs = map[string][2]string{
	"stats": {"yoke", "stats"},

	// optional/ — non-POSIX classic Unix/utility commands (Cluster 1).
	"atq":      {"optional", "atq"},
	"atrm":     {"optional", "atrm"},
	"cal":      {"optional", "cal"},
	"ncal":     {"optional", "cal"},
	"clip":     {"optional", "clip"},
	"duration": {"optional", "duration"},
	"gzip":     {"optional", "gzip"},
	"gunzip":   {"optional", "gzip"},
	"zcat":     {"optional", "gzip"},
	"hexdump":  {"optional", "hexdump"},
	"jq":       {"optional", "jq"},
	"ntp":      {"optional", "ntp"},
	"sntp":     {"optional", "ntp"},
	"tar":      {"optional", "tar"},
	"tree":     {"optional", "tree"},
	"tz":       {"optional", "tz"},
	"watch":    {"optional", "watch"},
	"which":    {"optional", "which"},
	"why":      {"optional", "why"},
	"wslpath":  {"optional", "winpath"},
	"cygpath":  {"optional", "winpath"},

	// optional/ — GNU-only coreutils extras (Cluster 2).
	"arch":      {"optional", "arch"},
	"b2sum":     {"optional", "b2sum"},
	"base32":    {"optional", "base32"},
	"base64":    {"optional", "base64"},
	"basenc":    {"optional", "basenc"},
	"chcon":     {"optional", "chcon"},
	"dir":       {"optional", "dir"},
	"dircolors": {"optional", "dircolors"},
	"factor":    {"optional", "factor"},
	"fmt":       {"optional", "fmt"},
	"groups":    {"optional", "groups"},
	"hostid":    {"optional", "hostid"},
	"hostname":  {"optional", "hostname"},
	"install":   {"optional", "install"},
	"link":      {"optional", "link"},
	"md5sum":    {"optional", "md5sum"},
	"mknod":     {"optional", "mknod"},
	"mktemp":    {"optional", "mktemp"},
	"nl":        {"optional", "nl"},
	"nproc":     {"optional", "nproc"},
	"numfmt":    {"optional", "numfmt"},
	"pinky":     {"optional", "pinky"},
	"printenv":  {"optional", "printenv"},
	"ptx":       {"optional", "ptx"},
	"readlink":  {"optional", "readlink"},
	"realpath":  {"optional", "realpath"},
	"seq":       {"optional", "seq"},
	"sha1sum":   {"optional", "sha1sum"},
	"sha224sum": {"optional", "sha224sum"},
	"sha256sum": {"optional", "sha256sum"},
	"sha384sum": {"optional", "sha384sum"},
	"sha512sum": {"optional", "sha512sum"},
	"shred":     {"optional", "shred"},
	"shuf":      {"optional", "shuf"},
	"stat":      {"optional", "stat"},
	"stdbuf":    {"optional", "stdbuf"},
	"sum":       {"optional", "sum"},
	"sync":      {"optional", "sync"},
	"tac":       {"optional", "tac"},
	"timeout":   {"optional", "timeout"},
	"truncate":  {"optional", "truncate"},
	"unlink":    {"optional", "unlink"},
	"uptime":    {"optional", "uptime"},
	"users":     {"optional", "users"},
	"vdir":      {"optional", "vdir"},
	"whoami":    {"optional", "whoami"},
	"yes":       {"optional", "yes"},

	// toolchain/ — self-provisioning third-party toolchains (Cluster 3).
	"go":      {"toolchain", "go"},
	"cmake":   {"toolchain", "cmake"},
	"clang":   {"toolchain", "clang"},
	"zig":     {"toolchain", "zig"},
	"node":    {"toolchain", "node"},
	"npm":     {"toolchain", "npm"},
	"npx":     {"toolchain", "npx"},
	"pnpm":    {"toolchain", "pnpm"},
	"yarn":    {"toolchain", "yarn"},
	"python":  {"toolchain", "python"},
	"pip":     {"toolchain", "pip"},
	"uv":      {"toolchain", "uv"},
	"mise":    {"toolchain", "mise"},
	"cargo":   {"toolchain", "cargo"},
	"rustc":   {"toolchain", "rustc"},
	"rustup":  {"toolchain", "rustup"},
	"rust":    {"toolchain", "rust"},
	"pwsh":    {"toolchain", "pwsh"},
	"git-scm": {"toolchain", "git-scm"},
	"curl":    {"toolchain", "curl"},

	// yoke/ — standalone Yoke applets & direct-constructor verbs (Cluster 4A).
	"ast":         {"yoke", "ast"},
	"browser":     {"yoke", "browser"},
	"dag":         {"yoke", "dag"},
	"db":          {"yoke", "db"},
	"fetch":       {"yoke", "fetch"},
	"foreman":     {"yoke", "foreman"},
	"graph":       {"yoke", "graph"},
	"lens":        {"yoke", "lens"},
	"lens-script": {"yoke", "lens"},
	"mb":          {"yoke", "mb"},
	"messages":    {"yoke", "mb"},
	"tokens":      {"yoke", "tokens"},
	"llm":         {"yoke", "llm"},
	"share":       {"yoke", "share"},
	"adapt":       {"yoke", "adapt"},
	"adapter":     {"yoke", "adapter"},
	"infra":       {"yoke", "infra"},
	"token":       {"yoke", "token"},
	"fleet":       {"yoke", "fleet"},
	"satellite":   {"yoke", "satellite"},
	"member":      {"yoke", "member"},
	"group":       {"yoke", "group"},
	"superadmin":  {"yoke", "superadmin"},
	"desktop":     {"yoke", "desktop"},
	"journey":     {"yoke", "journey"},
	"suite":       {"yoke", "suite"},
	"reproduce":   {"yoke", "reproduce"},
	"paired-host": {"yoke", "paired-host"},
	"resource":    {"yoke", "resource"},
	"resources":   {"yoke", "resource"},
	"handoff":     {"yoke", "handoff"},
	"resume":      {"yoke", "resume"},
	"todo":        {"yoke", "todo"},
	"issue":       {"yoke", "todo"},
	"pair":        {"yoke", "pair"},
	"judge":       {"yoke", "judge"},
	"gate":        {"yoke", "gate"},
	"coach":       {"yoke", "coach"},
	"delegate":    {"yoke", "delegate"},
	"supervise":   {"yoke", "supervise"},
	"capability":  {"yoke", "capability"},
	"chat":        {"yoke", "chat"},
	"invoke":      {"yoke", "chat"},
	"leaderboard": {"yoke", "leaderboard"},
	"sdlc":        {"yoke", "sdlc"},
	"web":         {"yoke", "web"},
	"weave":       {"yoke", "weave"},
	"sota":        {"yoke", "sota"},
	"account":     {"yoke", "account"},
	"secret":      {"yoke", "secret"},
	"secrets":     {"yoke", "secret"},
	"herald":      {"yoke", "herald"},
	"a2a":         {"yoke", "herald"},
	"acp":         {"yoke", "acp"},
	"ask":         {"yoke", "ask"},
	"mod":         {"yoke", "mod"},
	"schedule":    {"yoke", "schedule"},
	"outpost":     {"yoke", "outpost"},
	"peer":        {"yoke", "peer"},
	"sphere":      {"yoke", "peer"},
	"tessaro":     {"yoke", "tessaro"},
	"login":       {"yoke", "login"},
	"mirror":      {"yoke", "mirror"},

	// custom/ — binmgr-managed external CLIs (Cluster 5A).
	"loom":        {"custom", "loom"},
	"zot":         {"custom", "zot"},
	"seaweedfs":   {"custom", "seaweedfs"},
	"kopia":       {"custom", "kopia"},
	"act":         {"custom", "act"},
	"act-runner":  {"custom", "act-runner"},
	"gh":          {"custom", "gh"},
	"rclone":      {"custom", "rclone"},
	"kubectl":     {"custom", "kubectl"},
	"helm":        {"custom", "helm"},
	"govulncheck": {"custom", "govulncheck"},
	"osv-scanner": {"custom", "osv-scanner"},
	"osvscanner":  {"custom", "osv-scanner"},
	"grype":       {"custom", "grype"},

	// custom/ — declarative registry CLIs (Cluster 5B).
	"aws":           {"custom", "aws"},
	"az":            {"custom", "az"},
	"gcloud":        {"custom", "gcloud"},
	"glab":          {"custom", "glab"},
	"gws":           {"custom", "gws"},
	"doctl":         {"custom", "doctl"},
	"gitea":         {"custom", "gitea"},
	"karmadactl":    {"custom", "karmadactl"},
	"rg":            {"custom", "rg"},
	"tofu":          {"custom", "tofu"},
	"m365":          {"custom", "m365"},
	"gam":           {"custom", "gam"},
	"psql":          {"custom", "psql"},
	"mongosh":       {"custom", "mongosh"},
	"valkey-cli":    {"custom", "valkey-cli"},
	"snow":          {"custom", "snow"},
	"mosquitto":     {"custom", "mosquitto"},
	"rabbitmqadmin": {"custom", "rabbitmqadmin"},
	"kcat":          {"custom", "kcat"},
	"sf":            {"custom", "sf"},
	"stripe":        {"custom", "stripe"},
	"twilio":        {"custom", "twilio"},
	"slack":         {"custom", "slack"},

	// custom/ — lean external engine & observability shims (Cluster 5C).
	"podman":  {"custom", "podman"},
	"oci":     {"custom", "podman"},
	"sandbox": {"custom", "podman"},
	"docker":  {"custom", "podman"},
	"otel":    {"custom", "otel"},
}

func isAdaptedVerb(name string) bool {
	_, ok := adaptedVerbs[name]
	return ok
}

func lookupAdaptedVerb(name string) (folder, entry string, ok bool) {
	fe, ok := adaptedVerbs[name]
	if !ok {
		return "", "", false
	}
	return fe[0], fe[1], true
}

// customAgentCLIEntries lists the shipped baseline steerable agent CLIs that
// carry dedicated standalone adapters under bashy/custom/<tool>/. Any other
// steerable tool dynamically added via $BASHY_TOOLS_PATH / ~/.config/bashy/tools
// routes through bashy/custom/agent-cli/. Kept separate from adaptedVerbs so
// steerable tool shortcuts preserve their exact lowest-precedence dispatch slot
// after coreutils, registry, user-registered commands, and embedded skill docs.
var customAgentCLIEntries = map[string]bool{
	"claude":   true,
	"codex":    true,
	"agy":      true,
	"aider":    true,
	"dsh":      true,
	"muse":     true,
	"opencode": true,
	"zcode":    true,
}

func customAgentCLIEntry(toolName string) string {
	if customAgentCLIEntries[toolName] {
		return toolName
	}
	return "agent-cli"
}

// runCommandAdapter routes a front-door command through its folder-organized
// Bash# adapter (<folder>/<entry>/<entry>.bsh), passing argv, stdio, env, and
// exit status through unchanged. If an active shaped CLI slot exists for the
// command, it executes that slot directly (honouring bashy shape).
func runCommandAdapter(folder, entry, invokedAs string, args []string) int {
	if invokedAs == "" {
		invokedAs = entry
	}
	if !certProfile() {
		for _, slotName := range uniqueStrings(invokedAs, entry) {
			if active, ok, err := shapeSlotSelect(slotName); err == nil && ok && active.Path != "" {
				env, envErr := slot.EntryEnvironment(active.Path, appendAdapterEnv(os.Environ(), invokedAs, false))
				if envErr == nil {
					return shapeSlotLaunch(active.Path, append([]string{invokedAs}, args...), env)
				}
			}
		}
	}

	scriptPath := resolveAdapterScript(folder, entry)
	if scriptPath == "" {
		fmt.Fprintf(os.Stderr, "bashy %s: adapter %s/%s/%s.bsh not found\n", invokedAs, folder, entry, entry)
		return 127
	}

	self := bashySelfPath()
	cmdArgs := make([]string, 0, 2+len(args))
	cmdArgs = append(cmdArgs, "--bashsharp", scriptPath)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command(self, cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = appendAdapterEnv(os.Environ(), invokedAs, true)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return childExitStatus(exitErr)
		}
		fmt.Fprintf(os.Stderr, "bashy %s: %v\n", invokedAs, err)
		return 126
	}
	return 0
}

func uniqueStrings(a, b string) []string {
	if a == "" {
		return []string{b}
	}
	if b == "" || a == b {
		return []string{a}
	}
	return []string{a, b}
}

func appendAdapterEnv(base []string, invokedAs string, markAdapterExec bool) []string {
	self := bashySelfPath()
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case adapterExecEnv, invokedAsEnv, "BASHY_SELF":
			continue
		}
		out = append(out, kv)
	}
	if markAdapterExec {
		out = append(out, adapterExecEnv+"=1")
	}
	out = append(out, invokedAsEnv+"="+invokedAs)
	if self != "" {
		out = append(out, "BASHY_SELF="+self)
	}
	return out
}

func stripEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, prefix) {
			out = append(out, kv)
		}
	}
	return out
}

// resolveAdapterScript locates <folder>/<entry>/<entry>.bsh across explicit
// overrides, the fetched source pin (~/.bashy/src/bashy), the executable tree,
// the working directory, GOWORK, and compile-time source location (tests).
func resolveAdapterScript(folder, entry string) string {
	rel := filepath.Join(folder, entry, entry+".bsh")

	if override := strings.TrimSpace(os.Getenv(adaptersRootEnv)); override != "" {
		p := filepath.Join(override, rel)
		if isRegularFile(p) {
			return p
		}
		return ""
	}

	if !certProfile() {
		if home := strings.TrimSpace(src.Home()); home != "" {
			if p := filepath.Join(home, "src", "bashy", rel); isRegularFile(p) {
				return p
			}
		}
	}

	for _, root := range candidateBashyRoots() {
		if p := filepath.Join(root, rel); isRegularFile(p) {
			return p
		}
	}
	return ""
}

func candidateBashyRoots() []string {
	var seeds []string
	if exe, err := os.Executable(); err == nil && exe != "" {
		seeds = append(seeds, filepath.Dir(exe))
		if realExe, err := filepath.EvalSymlinks(exe); err == nil && realExe != "" && realExe != exe {
			seeds = append(seeds, filepath.Dir(realExe))
		}
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		seeds = append(seeds, wd, filepath.Join(wd, "bashy"))
	}
	if pwd := strings.TrimSpace(os.Getenv("PWD")); pwd != "" {
		seeds = append(seeds, pwd, filepath.Join(pwd, "bashy"))
	}
	if gowork := strings.TrimSpace(os.Getenv("GOWORK")); gowork != "" && gowork != "off" {
		seeds = append(seeds, filepath.Join(filepath.Dir(gowork), "bashy"))
	}
	if _, callerFile, _, ok := runtime.Caller(0); ok && filepath.IsAbs(callerFile) {
		seeds = append(seeds, filepath.Dir(callerFile))
	}

	seen := map[string]bool{}
	var roots []string
	for _, s := range seeds {
		if root, ok := findBashySourceRoot(s); ok && !seen[root] {
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}

func isRegularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
