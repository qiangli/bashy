// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// engineAlias normalizes a front-door engine alias to its canonical engine name.
// `bashy docker` is an alias for the podman engine — it is listed by
// `bashy commands`, so it must dispatch like any other verb (regression guard: it
// used to fall through to "docker: No such file or directory"). Shared by every
// dispatchEngine build variant (lean/full/windows) and unit-tested directly.
func engineAlias(name string) string {
	if name == "docker" {
		return "podman"
	}
	return name
}
