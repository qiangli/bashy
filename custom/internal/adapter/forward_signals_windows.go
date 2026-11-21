//go:build windows

package adapter

import "os"

func forwardTermSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
