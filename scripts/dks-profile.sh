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
#   . "$(dirname "${BASH_SOURCE[0]:-$0}")/dks-profile.sh"
#   KUBECTL="${KUBECTL:-$(dks_resolve_kubectl "outpost kubectl")}"
#
# An explicit $KUBECTL set by the caller's environment always wins over any
# profile - dks_resolve_kubectl is only consulted through the `${KUBECTL:-…}`
# fallback, exactly like the pre-profile default it replaces.
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
