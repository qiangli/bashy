#!/usr/bin/env bash
# dks-profile.sh - resolve $KUBECTL for the DKS emitter/result/gate scripts.
#
# Two profiles, selected by $DKS_PROFILE:
#
#   cloudbox (default, unchanged behavior) - KUBECTL resolves to the caller's
#     script-specific default (e.g. "outpost kubectl" or "bashy kubectl"),
#     which fetches a per-user kubeconfig FROM CLOUDBOX on demand.
#
#   peer - target a peer-hosted control plane's LOCAL kubeconfig instead.
#     Resolves $DKS_PEER_KUBECONFIG (default
#     $HOME/.kube/outpost-control-plane/k3s.yaml) and sets
#     KUBECTL="kubectl --kubeconfig <path>". Fails loudly (non-zero exit,
#     stderr message) when that file is absent - a wrong-cluster apply is the
#     failure mode this guards against, so peer never silently falls back to
#     cloudbox. Only the PATH is inspected; the kubeconfig content is never
#     read or printed by this script.
#
# Not sourced directly by users: each DKS script sources this file and calls
# dks_resolve_kubectl with its own existing default, e.g.:
#
#   . "$(cd "$(dirname "$0")" && pwd)/dks-profile.sh"
#   if [ -z "${KUBECTL:-}" ]; then
#     KUBECTL="$(dks_resolve_kubectl "outpost kubectl")"
#   fi
#
# Use the plain `$0` + `if [ -z ... ]; then KUBECTL=$(...); fi` form above,
# not the two patterns it replaced:
#
#   . "$(dirname "${BASH_SOURCE[0]:-$0}")/dks-profile.sh"
#   KUBECTL="${KUBECTL:-$(dks_resolve_kubectl "...")}"
#
# Both are valid POSIX/bash but panic Bashy's mvdan-based expander under
# `set -u`. Bashy never populates BASH_SOURCE[0] for the top-level script, so
# `${BASH_SOURCE[0]:-$0}` hits the nounset fallback path - and that fallback
# path nil-derefs in Bashy today. The same root cause hits any
# `${arr[i]:-$(cmd)}` array-index-with-command-substitution default,
# including the `KUBECTL="${KUBECTL:-$(...)}"` shorthand.
#
# See scripts/test-dks-profile.sh's bashy-panic regression, which executes
# every modified script through installed bashy (when available) and
# requires no panic.
#
# An explicit $KUBECTL set by the caller's environment always wins over any
# profile - dks_resolve_kubectl is only consulted when $KUBECTL is unset or
# empty, exactly like the pre-profile default it replaced. These scripts are
# always invoked directly (never sourced), so $0 and ${BASH_SOURCE[0]:-$0}
# name the same path under real bash - $0 alone is both simpler and
# Bashy-safe.
set -euo pipefail

dks_resolve_kubectl() {
  local default_kubectl="$1"
  local profile="${DKS_PROFILE:-cloudbox}"

  case "$profile" in
    cloudbox)
      printf '%s\n' "$default_kubectl"
      ;;
    peer)
      local kubeconfig="${DKS_PEER_KUBECONFIG:-$HOME/.kube/outpost-control-plane/k3s.yaml}"
      if [ ! -f "$kubeconfig" ]; then
        echo "dks-profile: DKS_PROFILE=peer but kubeconfig not found at $kubeconfig (set DKS_PEER_KUBECONFIG or restore the file) - refusing to fall back to cloudbox" >&2
        exit 9
      fi
      printf '%s\n' "kubectl --kubeconfig $kubeconfig"
      ;;
    *)
      echo "dks-profile: unknown DKS_PROFILE '$profile' (expected cloudbox or peer)" >&2
      exit 9
      ;;
  esac
}
