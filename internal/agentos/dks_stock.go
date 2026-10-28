// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || bashy_host || (!windows && !cgo && !containers_image_openpgp) || (windows && (!remote || !containers_image_openpgp))

package agentos

// dks_stock.go — the stock-build `bashy dks` client surface.
//
// The DKS VM provisioner needs the in-process podman machine libraries and
// only exists in the engine build (`-tags bashy_engines`), but agents on a
// stock build (CGO_ENABLED=0, all three OSes) still need the CLIENT-side
// operations: cluster status, kubeconfig (local or minted over the
// satellite), a shell over the satellite, and federation membership checks.
// Those ride the engine-free yoke packages (external/dks federation +
// userland, external/kube selection, external/meshagent exec) so this file
// adds no cgo, no podman engine, and no k3s to the stock binary.
//
// Routing lives in dispatchEngine (engines_stub.go): help/status/kubeconfig/
// shell are handled here, federation/userland delegate to yoke's dks package
// exactly as the engine build mounts it, and the VM verbs refuse with the
// gated message below.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/qiangli/yoke/external/kube"
	"github.com/qiangli/yoke/external/meshagent"
)

// stockDKSEngineMessage is the refusal for verbs that need the embedded
// engine. It keeps the historical "not available in this build" phrase (older
// tests and scripts match on it) and names the host/engine build explicitly.
const stockDKSEngineMessage = "bashy dks: not available in this build — " +
	"provisioning the DKS VM needs the host/engine build (`-tags bashy_engines`). " +
	"On Linux, run k3s under rootful podman directly (no VM)."

// stockDKSClientVerbs are the `bashy dks` subcommands the stock build serves
// itself (before federation/userland delegation and the engine refusal).
var stockDKSClientVerbs = []string{"status", "kubeconfig", "shell"}

// isStockDKSClient reports whether `bashy dks` args select a client verb
// handled by this file (as opposed to yoke's federation/userland packages or
// the engine-only VM verbs).
func isStockDKSClient(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, v := range stockDKSClientVerbs {
		if args[0] == v {
			return true
		}
	}
	return false
}

// isStockDKSHelp reports whether `bashy dks` args ask for top-level help.
func isStockDKSHelp(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "-h", "--help", "help":
		return true
	}
	return false
}

const stockDKSHelp = `bashy dks — DKS cluster client (stock build; the VM provisioner is not linked here)

CLIENT verbs — run anywhere, no engine build needed:
  status [--json]                 cluster-client status: kubeconfig plane, outpost agent, VM note
  kubeconfig [--refresh | --from-satellite [--node NAME] [--output PATH | --stdout]]
                                  show the kubeconfig kubectl/helm use; refresh it, or mint one over the satellite
  use @scope:<path>               select the matching scope namespace in the current kubeconfig context
  list [--json]                   list reachable scope namespaces; * marks the current namespace
  shell [--remote NAME] [-c CMD]  open a shell through the outpost agent (a satellite with --remote/--host)
  federation ...                  Karmada federation membership (join/leave/status/...) via the outpost cluster agent
  use|local|run|ps|logs|attach|stop|expose ...
                                  cluster-userland workspace verbs, same as the engine build

ENGINE verbs — need the host/engine build (` + "`-tags bashy_engines`" + `):
  provision                       ensure the dedicated rootful DKS VM exists and is running (macOS/Windows)

` + "`bashy dks status`" + ` and ` + "`bashy dks federation status`" + ` are the read-only checks agents need
for federation work; ` + "`provision`" + ` refuses here with a "needs the host/engine build" message.`

// dispatchStockDKSHelp prints the stock `bashy dks` help and returns exit 0.
// `bashy dks help <verb>` routes to that verb's own usage.
func dispatchStockDKSHelp(args []string) int {
	if len(args) == 2 && args[0] == "help" {
		switch args[1] {
		case "status":
			return runStockDKSStatus([]string{"--help"})
		case "kubeconfig":
			return runStockDKSKubeconfig([]string{"--help"})
		case "shell":
			return runStockDKSShell([]string{"--help"})
		case "federation":
			fmt.Fprint(os.Stdout, stockDKSHelp+"\n")
			fmt.Fprint(os.Stdout, "\nfederation details: `bashy dks federation --help`\n")
			return 0
		}
	}
	fmt.Fprint(os.Stdout, stockDKSHelp+"\n")
	return 0
}

// stockDKSStatus is the machine-readable shape of `bashy dks status --json`
// (bashy-dks-status-v1): what client plane this build would use, without
// provisioning, launching, or contacting anything.
type stockDKSStatus struct {
	Build            string `json:"build"`
	GOOS             string `json:"goos"`
	Kubeconfig       string `json:"kubeconfig"`
	KubeconfigSource string `json:"kubeconfig_source"`
	Outpost          bool   `json:"outpost"`
	OutpostPath      string `json:"outpost_path,omitempty"`
	VM               string `json:"vm"`
}

const stockDKSStatusHelp = `usage: bashy dks status [--json]   # read-only: cluster-client status (bashy-dks-status-v1)

Reports which kubeconfig plane ` + "`bashy kubectl`/`helm`" + ` would use, whether the
outpost satellite agent is reachable, and the VM note for this OS. Never
provisions, launches, or contacts a cluster, so it is safe on a stock build.`

// runStockDKSStatus implements `bashy dks status`: the client-side cluster
// status. Exit 0 after reporting (even with no cluster configured, mirroring
// the engine status "not provisioned" path); exit 2 on flag misuse; exit 1
// when kubeconfig selection fails closed.
func runStockDKSStatus(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help", "help":
			fmt.Fprintln(os.Stdout, stockDKSStatusHelp)
			return 0
		default:
			fmt.Fprintf(os.Stderr, "bashy dks status: unexpected argument %q (%s)\n", a, "usage: bashy dks status [--json]")
			return 2
		}
	}
	st := stockDKSStatus{Build: "stock", GOOS: runtime.GOOS}
	st.Kubeconfig, st.KubeconfigSource = stockDKSKubeconfigPlane()
	if resErr := stockDKSKubeconfigErr(); resErr != "" {
		if asJSON {
			st.KubeconfigSource = resErr
		} else {
			fmt.Fprintf(os.Stderr, "bashy dks status: %s\n", resErr)
			return 1
		}
		// JSON still reports what resolved alongside the error source note.
	}
	if path, ok := meshagent.Resolve(); ok {
		st.Outpost, st.OutpostPath = true, path
	}
	if runtime.GOOS == "linux" {
		st.VM = "none — Linux runs containers natively (no VM)"
	} else {
		st.VM = "unknown here — VM-machine state needs the host/engine build"
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			fmt.Fprintln(os.Stderr, "bashy dks status:", err)
			return 1
		}
		return 0
	}
	fmt.Printf("dks: kubeconfig %s (%s)\n", dashIfEmpty(st.Kubeconfig), st.KubeconfigSource)
	if st.Outpost {
		fmt.Printf("dks: outpost agent available (%s)\n", st.OutpostPath)
	} else {
		fmt.Println("dks: outpost agent not found on PATH — satellite shell/kubeconfig minting unavailable")
	}
	fmt.Printf("dks: vm: %s.\n", st.VM)
	fmt.Println("dks: federation membership: `bashy dks federation status`")
	return 0
}

// stockDKSKubeconfigPlane describes which kubeconfig the managed
// kubectl/helm would use, for display only. Resolution itself always goes
// through kube.ResolveKubeconfig; this mirrors its precedence to name the
// source. The second return is "" on success or the fail-closed error text.
func stockDKSKubeconfigPlane() (path, source string) {
	if v := strings.TrimSpace(os.Getenv("KUBECONFIG")); v != "" {
		return v, "KUBECONFIG env"
	}
	profile, profileSet := os.LookupEnv(kube.EnvProfile)
	profile = strings.TrimSpace(profile)
	if !profileSet {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return "", "standard ~/.kube/config"
		}
		res, err := kube.ResolveKubeconfig()
		if err != nil {
			return "", ""
		}
		return res.KUBECONFIG, "implicit cloudbox fallback"
	}
	res, err := kube.ResolveKubeconfig()
	if err != nil {
		return "", ""
	}
	return res.KUBECONFIG, kube.EnvProfile + "=" + profile
}

// stockDKSKubeconfigErr returns the fail-closed kubeconfig error text, or ""
// when selection succeeds.
func stockDKSKubeconfigErr() string {
	_, err := kube.ResolveKubeconfig()
	if err != nil {
		return err.Error()
	}
	return ""
}

const stockDKSKubeconfigHelp = `usage: bashy dks kubeconfig [--refresh | --from-satellite [--node NAME] [--output PATH | --stdout]]

Shows the kubeconfig ` + "`bashy kubectl`/`helm`" + ` would use (same selection, no network).
--refresh re-reads it through the outpost broker when the plane is refreshable.
--from-satellite mints a fresh kubeconfig over the satellite
(` + "`outpost cluster kubeconfig`" + `); --node selects the host, --output writes a
file, --stdout prints instead.`

// runStockDKSKubeconfig implements `bashy dks kubeconfig`: show, refresh, or
// mint-over-satellite the kubeconfig. Read-only by default; writes only with
// --refresh (broker refresh of the implicit file) or --from-satellite.
func runStockDKSKubeconfig(args []string) int {
	refresh := false
	fromSatellite := false
	node := ""
	output := ""
	stdout := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--refresh":
			refresh = true
		case a == "--from-satellite":
			fromSatellite = true
		case a == "--stdout":
			stdout = true
		case a == "-h" || a == "--help" || a == "help":
			fmt.Fprintln(os.Stdout, stockDKSKubeconfigHelp)
			return 0
		case a == "--node" && i+1 < len(args):
			i++
			node = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "--node="):
			node = strings.TrimSpace(strings.TrimPrefix(a, "--node="))
		case a == "--output" && i+1 < len(args):
			i++
			output = strings.TrimSpace(args[i])
		case strings.HasPrefix(a, "--output="):
			output = strings.TrimSpace(strings.TrimPrefix(a, "--output="))
		default:
			fmt.Fprintf(os.Stderr, "bashy dks kubeconfig: unexpected argument %q\n%s\n", a, stockDKSKubeconfigHelp)
			return 2
		}
	}
	if refresh && fromSatellite {
		fmt.Fprintln(os.Stderr, "bashy dks kubeconfig: --refresh and --from-satellite are exclusive")
		return 2
	}
	if stdout && output != "" {
		fmt.Fprintln(os.Stderr, "bashy dks kubeconfig: --stdout and --output are exclusive")
		return 2
	}
	if (node != "" || stdout || output != "") && !fromSatellite && !refresh {
		fmt.Fprintln(os.Stderr, "bashy dks kubeconfig: --node/--output/--stdout need --refresh or --from-satellite")
		return 2
	}
	if fromSatellite {
		return runStockDKSKubeconfigFromSatellite(node, output, stdout)
	}
	res, err := kube.ResolveKubeconfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks kubeconfig: %v\n", err)
		return 1
	}
	path, source := stockDKSKubeconfigPlane()
	if path == "" {
		path = res.KUBECONFIG
	}
	if refresh {
		if !res.Refresh || path == "" {
			fmt.Fprintf(os.Stderr, "bashy dks kubeconfig: plane %q is not broker-refreshable (only the implicit cloudbox file is)\n", source)
			return 1
		}
		if output != "" && output != path {
			fmt.Fprintln(os.Stderr, "bashy dks kubeconfig: --refresh writes the resolved file; use --from-satellite --output for a custom path")
			return 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		kube.RefreshKubeconfig(ctx, os.Stderr, path)
	}
	fmt.Println(path)
	return 0
}

// runStockDKSKubeconfigFromSatellite execs `outpost cluster kubeconfig` to
// mint a fresh kubeconfig over the satellite. Fail-closed with guidance when
// the outpost agent is absent.
func runStockDKSKubeconfigFromSatellite(node, output string, stdout bool) int {
	outArgs := []string{"cluster", "kubeconfig"}
	if node != "" {
		outArgs = append(outArgs, "--node", node)
	}
	switch {
	case stdout:
		outArgs = append(outArgs, "--stdout")
	case output != "":
		outArgs = append(outArgs, "--output", output)
	}
	err := meshagent.Exec(context.Background(), outArgs...)
	if errors.Is(err, meshagent.ErrNotFound) {
		fmt.Fprint(os.Stderr, stockDKSOutpostMissing("kubeconfig minting"))
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks kubeconfig: %v\n", err)
		return 1
	}
	return 0
}

const stockDKSShellHelp = `usage: bashy dks shell [--remote NAME | --host ADDR] [-c CMD] [-- args...]

Opens a shell through the outpost agent (` + "`outpost shell`" + ` passthrough): the local
paired shell by default, a satellite with --remote/--host. -c runs one command
and exits with its status. Needs the outpost agent; otherwise fail-closed with
guidance.`

// runStockDKSShell implements `bashy dks shell`: a thin passthrough to
// `outpost shell` so stock builds reach a shell over the satellite
// (--remote/--host). Help resolves locally so agents can discover the verb
// where no agent is installed.
func runStockDKSShell(args []string) int {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			fmt.Fprintln(os.Stdout, stockDKSShellHelp)
			return 0
		}
	}
	err := meshagent.Exec(context.Background(), append([]string{"shell"}, args...)...)
	if errors.Is(err, meshagent.ErrNotFound) {
		fmt.Fprint(os.Stderr, stockDKSOutpostMissing("shell"))
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks shell: %v\n", err)
		return 1
	}
	return 0
}

// stockDKSOutpostMissing is the fail-closed guidance when a satellite-backed
// verb has no outpost agent to drive.
func stockDKSOutpostMissing(verb string) string {
	return strings.Join([]string{
		"bashy dks: the outpost agent isn't available on this machine, so " + verb + " over the satellite is unavailable.",
		"",
		"Install the outpost agent (or put it on PATH / set $OUTPOST_BIN).",
		"To reach a satellite, pair first, then use --remote NAME (see `outpost remote login --help`).",
		"",
		"(Already have outpost installed? Put it on PATH or set $OUTPOST_BIN.)",
		"",
	}, "\n")
}
