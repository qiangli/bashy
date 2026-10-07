//go:build startup_signal_probe && (linux || darwin)

package main

import _ "github.com/qiangli/bashy/internal/startupsignalprobe"
