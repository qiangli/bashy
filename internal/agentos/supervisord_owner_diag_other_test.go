// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix && !linux

package agentos

import (
	"fmt"
	"net"
	"time"
)

// portDiagnosis separates an accepting listener from a refused bind.
func portDiagnosis(port string, _ ...int) string {
	dial, bind := "accepted", "ok"
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 250*time.Millisecond); err == nil {
		c.Close()
	} else {
		dial = err.Error()
	}
	if l, err := net.Listen("tcp", "127.0.0.1:"+port); err == nil {
		l.Close()
	} else {
		bind = err.Error()
	}
	return fmt.Sprintf("dial=%s bind=%s", dial, bind)
}
