// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

// Command kb-ui serves the local-knowledge monitor, explorer, and file preview UI.
// Independent of any external daemon: kb-ui invokes public bashy search/index APIs
// and serves an embedded offline asset bundle with security-sandboxed file previews.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:6674", "listen address for the UI server (loopback default)")
	rootsFlag := flag.String("roots", "", "comma-separated root paths for document search and preview")
	bashyBin := flag.String("bashy", "bashy", "path to bashy executable")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `kb-ui — local-knowledge monitor, explorer, and file preview for Bashy

Usage:
  kb-ui [--addr 127.0.0.1:6674] [--roots ~/notes,~/projects] [--bashy bashy]

Routes:
  /                serves the embedded SPA (explorer, monitor, queue, insights)
  /api/search      search indexed documents (invokes bashy search --indexed)
  /api/file        security-sandboxed file preview (eligible root documents only)
  /api/roots       manage indexed roots (visible includes/excludes/progress)
  /api/control     pause and resume index operations
  /events          reconnecting real SSE status stream
  /healthz         local liveness (HTTP 200)

`)
		flag.PrintDefaults()
	}
	flag.Parse()

	var initialRoots []string
	if *rootsFlag != "" {
		for _, r := range strings.Split(*rootsFlag, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				initialRoots = append(initialRoots, r)
			}
		}
	} else {
		// Default to current working directory if no roots specified
		if cwd, err := os.Getwd(); err == nil {
			initialRoots = append(initialRoots, cwd)
		}
	}

	rootsMgr := NewRootsManager(initialRoots)
	adapter := NewBashyCLIAdapter(*bashyBin, rootsMgr)
	hub := NewSSEHub()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	hub.StartHeartbeat(ctx, 15*time.Second)

	if err := serve(ctx, *addr, adapter, hub); err != nil {
		fmt.Fprintln(os.Stderr, "kb-ui:", err)
		os.Exit(1)
	}
}
