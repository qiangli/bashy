//go:build ownedexecprobe

package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/interp/ownedexec"
)

// This helper is compiled only by the focused owned-exec integration test.
// It observes the inherited OS descriptor directly, independently of shell
// read -u behavior, while using the production frame decoder.
func init() {
	if len(os.Args) != 2 || os.Args[1] != ownedexec.Sentinel {
		return
	}
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) != 4 || os.Args[1] != "-c" || os.Args[2] != ":" || len(os.Args[3]) != 200000 {
		return
	}
	fd := uintptr(9)
	if runtime.GOOS == "windows" {
		fd = 0
		for _, entry := range strings.Split(os.Getenv("BASHY_INHERITED_HANDLES"), ",") {
			parts := strings.Split(entry, ":")
			if len(parts) == 3 && parts[0] == "9" {
				n, err := strconv.ParseUint(parts[2], 0, 64)
				if err == nil {
					fd = uintptr(n)
				}
			}
		}
	}
	if fd == 0 {
		fmt.Fprintln(os.Stderr, "missing inherited fd 9")
		os.Exit(1)
	}
	f := os.NewFile(fd, "owned-exec-fd-9")
	if f == nil {
		fmt.Fprintln(os.Stderr, "cannot adopt inherited fd 9")
		os.Exit(1)
	}
	line, err := bufio.NewReader(f).ReadString('\n')
	if err != nil || line != "pipe-fd\n" {
		fmt.Fprintf(os.Stderr, "fd 9: line=%q error=%v\n", line, err)
		os.Exit(1)
	}
	if len(os.Args) < 2 || len(os.Args[len(os.Args)-1]) != 200000 || len(os.Getenv("v")) != 200000 || os.Getenv(ownedexec.Marker) != "" {
		fmt.Fprintln(os.Stderr, "frame argv/env/marker mismatch")
		os.Exit(1)
	}
	fmt.Println("fd-ok")
	os.Exit(0)
}
