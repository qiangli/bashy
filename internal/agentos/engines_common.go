// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"strings"
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
	if name == "docker" || name == "sandbox" {
		return "podman"
	}
	return name
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
func engineCacheDir() string {
	if d := strings.TrimSpace(os.Getenv("BASHY_BIN_CACHE")); d != "" {
		return d
	}
	cb, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cb, "bashy", "bin")
}
