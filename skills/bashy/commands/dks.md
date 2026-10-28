---
name: dks
description: Select a DKS workspace or scope namespace, inspect the issued kubeconfig, and list reachable scope namespaces.
---

# DKS client context

`bashy dks use` still selects a workspace by name or `@ws:<id>`. Use
`bashy dks use @scope:<path>` to set the current kubeconfig context's default
namespace to the projected scope name. For example,
`@scope:grade-10-9/a` selects `scope-grade-10-9-a`. Bare `bashy dks use`
shows the scope namespace when the current context uses a `scope-*` namespace.

`bashy dks list [--json]` lists `scope-*` namespaces reachable by the
account in that kubeconfig. It marks the current namespace with `*` in text
output or `current: true` in JSON. Listing requires the account to be able
to list namespaces and request Kubernetes self access reviews. The scope
selector changes only the active context in one kubeconfig file; with a
multi-file `KUBECONFIG`, select one file first.

`bashy dks kubeconfig` reports the selected file. Use its existing
`--from-satellite` option to mint the issued kubeconfig before switching
namespaces on a fresh client.
