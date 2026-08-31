//go:build linux

package agentos

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func inboxAncestrySupported() bool { return true }

func inboxParentProcessID(pid int) (int, bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	// comm is parenthesized and may itself contain spaces or ')'. The fields
	// after its FINAL ')' begin with state, then ppid.
	stat := string(b)
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	return ppid, err == nil
}
