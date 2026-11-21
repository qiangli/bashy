// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qiangli/yoke/pkg/binmgr"
)

func run(args []string) int {
	bin := resolvePodmanBinary()
	if bin == "" {
		bin = provisionPodman(context.Background())
	}
	if bin != "" {
		applyPodmanHelperEnv()
		applyManagedPodmanEnv(bin)
		return execEnginePassthrough(bin, args)
	}
	fmt.Fprint(os.Stderr, podmanNotFoundMessage())
	return 127
}

func podmanNotFoundMessage() string {
	return "bashy podman: no podman found on PATH or in bashy's cache, and none could be fetched.\n" +
		"  Check network access to github.com releases (bashy fetches a pinned podman for " + binmgr.Platform() + "),\n" +
		"  install podman yourself, or run containers on a paired host node over the mesh.\n"
}

func main() {
	os.Exit(run(os.Args[1:]))
}
