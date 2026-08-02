#!/usr/bin/env bash
# dks-profile.sh - resolve the kubectl invocation for the DKS
# emitter/result/gate scripts, as an ARGV-SAFE argv rather than a command
# string.
#
# Each DKS script sources this file and calls `dks_kubectl_init <default>` once
# at startup, which populates the global array DKS_KUBECTL_CMD. Thereafter the
# script runs kubectl via `dks_kubectl <args...>` (or, when a value must be
# captured, `dks_kubectl ... 2>/dev/null`). Nothing ever word-splits or globs a
# command string, so a kubeconfig path containing a space, `*`, `?`, or `[abc]`
# is passed through literally.
#
#   . "$(cd "$(dirname "$0")" && pwd)/dks-profile.sh"
#   dks_kubectl_init "outpost kubectl" || exit $?
#   ...
#   job_uid="$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.metadata.uid}')"
#
# Use the plain `$0`-based self-location above, never `${BASH_SOURCE[0]:-$0}`:
# Bashy never populates BASH_SOURCE[0] for the top-level script, so that idiom
# hits an array-index/`:-` fallback that nil-derefs (SIGSEGV) in Bashy's
# mvdan-based expander under `set -u`. These scripts are always invoked directly
# (never sourced), so `$0` names the same path under real bash and is both
# simpler and Bashy-safe. For the same reason resolution never uses the
# `VAR="${VAR:-$(cmd)}"` array/command-substitution default shorthand.
#
# ---------------------------------------------------------------------------
# Resolution precedence (dks_kubectl_init):
#
#   1. $DKS_KUBECTL_ARGV - a newline-serialized argv handed down from a PARENT
#      DKS script (dks-release-gate/dks-author-qa-refs -> their children). This
#      is the argv-safe propagation channel: each element is one line, so a
#      kubeconfig path with whitespace/glob metacharacters survives the process
#      boundary intact (an env string could not carry it unambiguously).
#      Serialize with `dks_kubectl_serialize`.
#
#   2. $KUBECTL - the legacy STRING override (operator-set, and what the
#      existing evidence suites pass). Kept for backwards compatibility. It is
#      split on spaces with globbing DISABLED and exec'd as a proper argv, so it
#      is glob-safe; it cannot represent a path containing a space (use a peer
#      profile / $DKS_PEER_KUBECONFIG for that). An explicit $KUBECTL still wins
#      over any $DKS_PROFILE, exactly as before.
#
#   3. $DKS_PROFILE - the profile selector:
#        cloudbox (default)  the caller's own default argv (e.g. "outpost
#                            kubectl" / "bashy kubectl"), which fetches a
#                            per-user kubeconfig FROM CLOUDBOX on demand.
#        peer                target a peer control plane's LOCAL kubeconfig:
#                            resolves $DKS_PEER_KUBECONFIG (default
#                            $HOME/.kube/outpost-control-plane/k3s.yaml) and
#                            builds `kubectl --kubeconfig <path>` with the path
#                            carried as its OWN argv element. Fails closed
#                            (returns 9, stderr message naming the path) when
#                            that file is absent or unreadable - a wrong-cluster
#                            apply is the failure mode this guards, so peer never
#                            silently falls back to cloudbox.
#
#      An explicitly-EMPTY DKS_PROFILE (e.g. a typo'd `DKS_PROFILE=$UNSET_VAR`)
#      is NOT treated as unset: it fails closed as an unknown profile rather than
#      quietly routing to cloudbox. Only a genuinely unset DKS_PROFILE defaults
#      to cloudbox (the `${DKS_PROFILE-cloudbox}` single-dash form).
#
# SECRET SAFETY: this session may run under unredacted PTY capture, so anything
# printed lands in an on-disk log. A kubectl argv can carry `--token=`,
# `--password=`, a bearer token, or a URL with embedded credentials, so this
# file NEVER echoes the resolved argv, the $KUBECTL string, or $DKS_KUBECTL_ARGV
# - not on the success path, not on any error path. Announcements name only the
# PROFILE plus a non-secret descriptor (the kubeconfig PATH, or the executable
# BASENAME). The kubeconfig content is likewise never read or printed.
#
# See scripts/test-dks-profile.sh for the token-non-leak, whitespace, glob, and
# Bashy-panic regressions, and docs/plan-dks-build-test-deploy.md for the design.
set -euo pipefail

# The resolved kubectl argv. Populated by dks_kubectl_init.
DKS_KUBECTL_CMD=()

# _dks_announce PROFILE DESCRIPTOR - safe, non-secret stderr announcement of the
# resolved target. DESCRIPTOR must already be a non-secret token (a kubeconfig
# path or an executable basename); callers never pass an argv/override/token.
_dks_announce() {
  printf 'dks-profile: kubectl profile=%s %s\n' "$1" "$2" >&2
}

# _dks_basename EXEC - executable basename via parameter expansion (no external
# `basename`, whose macOS build predates `--`). A non-secret descriptor: only
# the program name, never its argv.
_dks_basename() {
  local e="${1:-kubectl}"
  printf '%s' "${e##*/}"
}

# NOTE: the argv split is done INLINE at each use site rather than in a helper.
# Populating a caller's array from a function needs `declare -n`, which the macOS
# system /bin/bash (3.2) lacks - and BOTH /bin/bash and bashy must pass. Inline
# `set -f; IFS=' ' read -r -a DKS_KUBECTL_CMD <<<"$str"; set +f` splits on spaces
# with globbing disabled: controlled defaults and the legacy $KUBECTL string only
# (a path element cannot contain a space here; a `*`/`?`/`[` in it is preserved).

# dks_kubectl - run the resolved kubectl argv, argv-safe: the stored elements are
# quoted (no word-splitting, no globbing), and $@ is passed through verbatim.
dks_kubectl() {
  "${DKS_KUBECTL_CMD[@]}" "$@"
}

# dks_kubectl_serialize - print the resolved argv, one element per line, for
# argv-safe hand-off to a child DKS script via DKS_KUBECTL_ARGV. Emits nothing
# if the argv is empty (never called before dks_kubectl_init in practice).
dks_kubectl_serialize() {
  [ "${#DKS_KUBECTL_CMD[@]}" -gt 0 ] || return 0
  printf '%s\n' "${DKS_KUBECTL_CMD[@]}"
}

# dks_kubectl_init DEFAULT_CMD - resolve DKS_KUBECTL_CMD per the precedence above.
# Returns 0 on success, 9 on a fail-closed resolution error (peer-missing /
# unknown profile). Callers propagate with `dks_kubectl_init "..." || exit $?`.
dks_kubectl_init() {
  local default_cmd="$1"

  # 1. Inherited argv-safe propagation from a parent DKS script.
  if [ -n "${DKS_KUBECTL_ARGV:-}" ]; then
    local line
    DKS_KUBECTL_CMD=()
    while IFS= read -r line; do
      # Skip empty lines: `printf '%s\n'` serializers end with a trailing
      # newline, and a hand-exported DKS_KUBECTL_ARGV may too. That trailing
      # newline would otherwise reconstruct as a spurious empty argv element
      # (an empty element is unrepresentable in the legacy $KUBECTL string
      # channel anyway, so dropping it loses nothing).
      [ -n "$line" ] || continue
      DKS_KUBECTL_CMD+=("$line")
    done <<<"$DKS_KUBECTL_ARGV"
    _dks_announce inherited "exec=$(_dks_basename "${DKS_KUBECTL_CMD[0]:-}")"
    return 0
  fi

  # 2. Legacy string override - wins over any profile.
  if [ -n "${KUBECTL:-}" ]; then
    set -f
    IFS=' ' read -r -a DKS_KUBECTL_CMD <<<"$KUBECTL"
    set +f
    _dks_announce override "exec=$(_dks_basename "${DKS_KUBECTL_CMD[0]:-}")"
    return 0
  fi

  # 3. Profile. Single-dash: an explicitly-empty value is NOT unset.
  local profile="${DKS_PROFILE-cloudbox}"
  case "$profile" in
    cloudbox)
      set -f
      IFS=' ' read -r -a DKS_KUBECTL_CMD <<<"$default_cmd"
      set +f
      _dks_announce cloudbox "exec=$(_dks_basename "${DKS_KUBECTL_CMD[0]:-}")"
      return 0
      ;;
    peer)
      local kubeconfig="${DKS_PEER_KUBECONFIG:-$HOME/.kube/outpost-control-plane/k3s.yaml}"
      if [ ! -f "$kubeconfig" ] || [ ! -r "$kubeconfig" ]; then
        echo "dks-profile: DKS_PROFILE=peer but kubeconfig not found or unreadable at $kubeconfig (set DKS_PEER_KUBECONFIG or restore the file) - refusing to fall back to cloudbox" >&2
        return 9
      fi
      # Path carried as its own argv element: whitespace/glob-safe.
      DKS_KUBECTL_CMD=(kubectl --kubeconfig "$kubeconfig")
      _dks_announce peer "kubeconfig=$kubeconfig"
      return 0
      ;;
    *)
      echo "dks-profile: unknown DKS_PROFILE '$profile' (expected cloudbox or peer)" >&2
      return 9
      ;;
  esac
}
