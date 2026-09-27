package agentos

// Sprint: #301; Story: #957; Story-ID: e5f503a3831a

import (
	"os"
	"strings"
)

// podmanStartsMachine reports whether args start a podman machine. The VM and
// its helper daemons (WSL and win-sshproxy on Windows, vfkit and gvproxy on
// macOS) outlive the command and inherit its working directory; on Windows a
// directory that a live process has as its cwd cannot be deleted, so a machine
// started from a work tree pins that tree until the machine stops.
func podmanStartsMachine(args []string) bool {
	// podman's subcommand is its first word that is neither a flag nor a
	// flag's value; "machine" is never a value, so it ends the scan.
	var words []string
	afterFlag := false
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-"):
			afterFlag = !strings.Contains(a, "=")
		case afterFlag && a != "machine":
			afterFlag = false
		default:
			words = append(words, a)
			afterFlag = false
		}
	}
	return len(words) >= 2 && words[0] == "machine" && words[1] == "start"
}

// engineDaemonDir is the working directory long-lived engine daemons start
// in: the user's home, which outlives any work tree, else the temp dir.
func engineDaemonDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return os.TempDir()
}
