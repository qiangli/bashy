package agentos

import "strings"

// mapNullDevice rewrites POSIX /dev/null arguments to the Windows NUL device
// for host programs (curl -o /dev/null, --output=/dev/null) that open the
// path themselves instead of receiving a shell redirection.
func mapNullDevice(goos string, args []string) []string {
	if goos != "windows" {
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case a == "/dev/null":
			a = "NUL"
		case strings.HasPrefix(a, "--") && strings.HasSuffix(a, "=/dev/null"):
			a = strings.TrimSuffix(a, "/dev/null") + "NUL"
		}
		out[i] = a
	}
	return out
}
