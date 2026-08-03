# DKS node-selection fixtures

Frozen, offline stand-ins for the three cluster reads
`scripts/dks-select-node.sh` performs. Every one of them is a trimmed but
structurally faithful capture of a real payload:

| file | stands in for |
|---|---|
| `nodes*.json` | `kubectl get nodes -o json` |
| `pods*.json` | `kubectl get pods --all-namespaces -o json` |
| `metrics*.json` | `kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes` |

`scripts/test-dks-select-node.sh` drives the selector against these and nothing
else — no cluster, no `kubectl` execution, no network. Fields the selector does
not read (managedFields, resourceVersion, images, nodeInfo, …) are omitted; the
fields it does read are spelled exactly as Kubernetes spells them, including the
unit vocabulary that makes this worth testing at all: allocatable CPU as `10` or
`7900m`, allocatable memory as `32Gi`, and metrics CPU in **nanocores**
(`420000000n`) with metrics memory in **Ki**.

The node inventory in `nodes.json` deliberately contains one of every exclusion
the selector must make — not-Ready, cordoned, untainted-but-untolerated,
wrong-OS, wrong-backend, wrong-arch, and too-small — so a filter that quietly
stops filtering shows up as a changed selection rather than as nothing at all.
