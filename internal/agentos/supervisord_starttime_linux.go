// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import "os"

// hostProc reads the real /proc; supervisord_procstat.go holds the liveness
// decisions.
var hostProc = procFS{readFile: os.ReadFile, readDir: os.ReadDir}

// procStartTime names a live process's incarnation (procFS.startTime).
func procStartTime(pid int) (string, error) { return hostProc.startTime(pid) }

// procStatFields returns /proc stat fields from field 3 (state) on.
func procStatFields(path string) ([]string, error) { return hostProc.stat(path) }

func processGroupIdentities(pgid int) ([]processIdentity, error) {
	return hostProc.groupIdentities(pgid)
}
