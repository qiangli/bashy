// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/qiangli/yoke/pkg/policy/coord"
)

// engineAlias normalizes a front-door engine alias to its canonical engine name.
// `bashy docker` is an alias for the podman engine — it is listed by
// `bashy commands`, so it must dispatch like any other verb (regression guard: it
// used to fall through to "docker: No such file or directory"). Shared by every
// dispatchEngine build variant (lean/full/windows) and unit-tested directly.
func engineAlias(name string) string {
	// `sandbox` is the tier-3 NAME, aliased so the tier vocabulary and the verb
	// surface agree. It is the RAW local engine and refuses nothing — outpost's
	// `sandbox` app is a filtered libpod endpoint that strips privileged/host-
	// namespace/host-bind/added-cap requests, and this alias does not inherit
	// those guarantees. Do not narrow one without narrowing the other.
	// `oci` is the STANDARD (canonical) name, `sandbox` the popular one, and
	// `docker`/`podman` the vendor spellings; all four run the podman engine.
	if name == "oci" || name == "docker" || name == "sandbox" {
		return "podman"
	}
	return name
}

// podmanMachineName extracts the target machine name from podman/oci/sandbox arguments,
// defaulting to "bashy".
func podmanMachineName(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--connection" || a == "-c" {
			if i+1 < len(args) {
				return args[i+1]
			}
		} else if strings.HasPrefix(a, "--connection=") {
			return strings.TrimPrefix(a, "--connection=")
		} else if strings.HasPrefix(a, "-c=") {
			return strings.TrimPrefix(a, "-c=")
		} else if a == "--machine" {
			if i+1 < len(args) {
				return args[i+1]
			}
		} else if strings.HasPrefix(a, "--machine=") {
			return strings.TrimPrefix(a, "--machine=")
		}
	}
	for i := 0; i < len(args); i++ {
		if args[i] == "machine" && i+2 < len(args) {
			sub := args[i+1]
			name := args[i+2]
			if !strings.HasPrefix(sub, "-") && !strings.HasPrefix(name, "-") {
				return name
			}
		}
	}
	if v := strings.TrimSpace(os.Getenv("CONTAINER_CONNECTION")); v != "" {
		return v
	}
	return "bashy"
}

// ollamaInstanceName extracts the target instance name from ollama arguments,
// defaulting to "bashy".
func ollamaInstanceName(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--instance" {
			if i+1 < len(args) {
				return args[i+1]
			}
		} else if strings.HasPrefix(a, "--instance=") {
			return strings.TrimPrefix(a, "--instance=")
		}
	}
	if v := strings.TrimSpace(os.Getenv("BASHY_OLLAMA_INSTANCE")); v != "" {
		return v
	}
	return "bashy"
}

// guardSandbox guards the podman/oci/sandbox front door.
// Returns 0 to proceed, or coordExitRefused (9) on conflict.
func guardSandbox(args []string) int {
	if !coordEnabled() {
		return 0
	}
	machine := podmanMachineName(args)
	if err := coord.Guard(context.Background(), coord.Self(), coord.Use{Kind: "sandbox", Name: machine}); err != nil {
		var conf *coord.Conflict
		if errors.As(err, &conf) {
			fmt.Fprint(os.Stderr, conf.Error())
			return coordExitRefused
		}
		fmt.Fprintln(os.Stderr, err)
		return coordExitRefused
	}
	return 0
}

// guardOllama guards the managed ollama front door.
// Returns 0 to proceed, or coordExitRefused (9) on conflict.
func guardOllama(args []string) int {
	if !coordEnabled() {
		return 0
	}
	instance := ollamaInstanceName(args)
	if err := coord.Guard(context.Background(), coord.Self(), coord.Use{Kind: "ollama", Name: instance}); err != nil {
		var conf *coord.Conflict
		if errors.As(err, &conf) {
			fmt.Fprint(os.Stderr, conf.Error())
			return coordExitRefused
		}
		fmt.Fprintln(os.Stderr, err)
		return coordExitRefused
	}
	return 0
}

// guardEngine runs the front-door guard for container and LLM engines.
func guardEngine(name string, args []string) int {
	switch engineAlias(name) {
	case "podman":
		return guardSandbox(args)
	case "ollama":
		return guardOllama(args)
	}
	return 0
}

// ollamaCloudTarget reports whether an `ollama` invocation targets ollama.com's
// HOSTED CLOUD rather than the local runtime: a ":cloud"-suffixed model (e.g.
// `run glm-5.2:cloud`) or the account verbs `signin`/`signout`. bashy ollama is
// the ISOLATED, SELF-HOSTED runtime (own port/store, mesh-shareable), so by
// default it refuses to drop the user into ollama.com's sign-in wall.
func ollamaCloudTarget(args []string) (target string, isCloud bool) {
	for _, a := range args {
		la := strings.ToLower(strings.TrimSpace(a))
		switch la {
		case "signin", "signout":
			return la, true
		}
		// ollama.com cloud models tag the reference "cloud": either exactly
		// `<model>:cloud` (e.g. glm-5.2:cloud) or `<model>:<size>-cloud` (e.g.
		// gpt-oss:120b-cloud). Match on the tag, not the whole arg, so unrelated
		// args ending in "cloud" don't misfire.
		if i := strings.LastIndex(la, ":"); i >= 0 {
			if tag := la[i+1:]; tag == "cloud" || strings.HasSuffix(tag, "-cloud") {
				return a, true
			}
		}
	}
	return "", false
}

// ollamaCloudAllowed is the opt-in escape hatch for users who really want
// ollama.com's hosted cloud: BASHY_OLLAMA_ALLOW_CLOUD truthy.
func ollamaCloudAllowed() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_OLLAMA_ALLOW_CLOUD"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// ollamaCloudBlockMessage explains why bashy ollama declined an ollama.com cloud
// request and how to stay self-hosted (or opt in).
func ollamaCloudBlockMessage(target string) string {
	what := "\"" + target + "\" is an ollama.com CLOUD request"
	if target == "signin" || target == "signout" {
		what = "`ollama " + target + "` targets an ollama.com account"
	}
	return "bashy ollama: " + what + " — it runs on ollama.com's hosted service and\n" +
		"needs a personal ollama.com sign-in. bashy ollama is your ISOLATED, SELF-HOSTED\n" +
		"runtime (local models, own port/store, mesh-shareable), so it won't sign you in.\n\n" +
		"  • Local:   bashy ollama pull <model> && bashy ollama run <model>\n" +
		"  • Shared:  run it on a paired host / the pooled-LLM gateway over the mesh\n" +
		"  • Opt in:  BASHY_OLLAMA_ALLOW_CLOUD=1 bashy ollama <args>   (use ollama.com cloud)\n"
}

// ollamaCloudGate prints the guidance and returns true when a cloud request must
// be refused (cloud target + no opt-in). Shared by every dispatch variant.
func ollamaCloudGate(args []string) (blocked bool, message string) {
	if t, cloud := ollamaCloudTarget(args); cloud && !ollamaCloudAllowed() {
		return true, ollamaCloudBlockMessage(t)
	}
	return false, ""
}

// These two are NOT engine-specific: they name bashy's managed-binary cache and
// the release repo those managed blobs come from. The OBSERVABILITY stack is a
// managed download through the same binmgr, so obs_stub.go needs them with no
// opinion about engines — and it lives behind !bashy_obs while their old home
// (engines_stub.go) lives behind !bashy_engines. Those two tags are
// INDEPENDENT, so `BASHY_ENGINES=1` without `BASHY_OBS` dropped the
// definitions and kept a caller. Untagged is the only home that satisfies all
// four combinations.

// engineReleaseRepo is the repo whose release carries bashy's permissive engine
// blobs. Overridable via $BASHY_ENGINE_REPO for forks/mirrors.
func engineReleaseRepo() string {
	if r := strings.TrimSpace(os.Getenv("BASHY_ENGINE_REPO")); r != "" {
		return r
	}
	return "qiangli/bashy"
}

// engineCacheDir is bashy's managed-binary cache — $BASHY_BIN_CACHE if set (as
// binmgr honors it), else <UserCacheDir>/bashy/bin.
//
// binmgr owns the cache; this is a name for its resolver, not a second copy.
// An empty return means no cache dir could be determined.
func engineCacheDir() string {
	d, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	return d
}

// ollamaStatusSchemaVersion is the envelope `bashy ollama status --json`
// emits: how this build reaches ollama (embedded, lean passthrough, or
// unsupported), whether a binary is already available without provisioning,
// and the isolation settings the managed engine would run under. It is a
// read-only probe — no download, no daemon, no network — so it is the safe
// JSON surface of a verb whose other subcommands all reach the engine.
const ollamaStatusSchemaVersion = "bashy-ollama-status-v1"

// ollamaEngineProbe is what each build variant knows about its ollama
// without touching it (engines_stub.go, engines_full.go, engines_windows.go).
type ollamaEngineProbe struct {
	Build         string // embedded | lean | unsupported
	Binary        string // resolved executable, "" when none is available yet
	Provisionable bool   // the lean build can fetch the official release here
}

type ollamaStatusEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	Engine        string `json:"engine"`
	Build         string `json:"build"`
	Binary        string `json:"binary,omitempty"`
	Available     bool   `json:"available"`
	Provisionable bool   `json:"provisionable"`
	ModelsDir     string `json:"models_dir,omitempty"`
	Host          string `json:"host,omitempty"`
	CloudAllowed  bool   `json:"cloud_allowed"`
	Door          string `json:"door"`
}

func ollamaStatus() ollamaStatusEnvelope {
	probe := probeOllamaEngine()
	env := ollamaStatusEnvelope{SchemaVersion: ollamaStatusSchemaVersion, Engine: "ollama", Build: probe.Build,
		Binary: probe.Binary, Available: probe.Binary != "", Provisionable: probe.Provisionable,
		Host: strings.TrimSpace(os.Getenv("OLLAMA_HOST")), CloudAllowed: ollamaCloudAllowed(), Door: "bashy llm"}
	if env.ModelsDir = strings.TrimSpace(os.Getenv("OLLAMA_MODELS")); env.ModelsDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			env.ModelsDir = filepath.Join(home, ".agents", "bashy", "ollama", "models")
		}
	}
	return env
}

// runOllamaStatus answers `bashy ollama status [--json]`.
func runOllamaStatus(args []string) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "--help", "-h":
			fmt.Println("usage: bashy ollama status [--json]   # read-only: how this build reaches ollama (bashy-ollama-status-v1)")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "bashy ollama status: unexpected argument %q (usage: bashy ollama status [--json])\n", a)
			return 2
		}
	}
	env := ollamaStatus()
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(env); err != nil {
			fmt.Fprintln(os.Stderr, "bashy ollama status:", err)
			return 1
		}
		return 0
	}
	available := "no"
	if env.Available {
		available = "yes"
	}
	fmt.Printf("ollama: build=%s available=%s binary=%s models=%s host=%s door=%s\n",
		env.Build, available, dashIfEmpty(env.Binary), env.ModelsDir, dashIfEmpty(env.Host), env.Door)
	return 0
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
