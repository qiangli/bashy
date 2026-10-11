// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin && !windows

package agentos

// procStartTime: no portable start-time source here, so ownership records are
// never written and nothing is reclaimed.
func procStartTime(int) (string, error) { return "", errStartTimeUnsupported }
