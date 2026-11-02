// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin

package agentos

// Other platforms retain direct-child ownership; no descendant guarantee.
func processGroupIdentities(pid int) ([]processIdentity, error) {
	start, err := procStartTime(pid)
	if err != nil {
		return nil, err
	}
	return []processIdentity{{pid, start}}, nil
}
