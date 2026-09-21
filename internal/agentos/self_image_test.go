// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"
)

// The image is one static binary and nothing else; HOME is its own dir under
// /tmp (Stage 0 canonicalizes the home prefix on non-tty output, and /tmp as
// HOME would rewrite every /tmp path a script prints).
func TestScratchContainerfileShape(t *testing.T) {
	cf := scratchContainerfile("v1.2.3")
	for _, want := range []string{"FROM scratch\n", "COPY bashy /bashy\n", "HOME=/tmp/bashy", "OTEL_TRACES_EXPORTER=none", "BASHY_TELEMETRY_QUIET=1", "WORKDIR /work\n", `ENTRYPOINT ["/bashy"]`, `image.version="v1.2.3"`} {
		if !strings.Contains(cf, want) {
			t.Errorf("Containerfile missing %q:\n%s", want, cf)
		}
	}
	if strings.Contains(cf, "RUN ") {
		t.Errorf("FROM scratch has no shell to RUN anything:\n%s", cf)
	}
}

// self fetch/install must never pick the image artifact as a shell release.
func TestScratchAssetNotAShellRelease(t *testing.T) {
	if bashyArchiveMatch("bashy-scratch-linux-amd64", "linux", "amd64") {
		t.Fatal("bashy-scratch-linux-amd64 matched as a bashy release archive")
	}
	if !bashyArchiveMatch("bashy-linux-amd64.tar.gz", "linux", "amd64") {
		t.Fatal("the real release archive stopped matching")
	}
}

func TestEngineArgvOverride(t *testing.T) {
	argv, err := engineArgv("docker --context x")
	if err != nil || strings.Join(argv, " ") != "docker --context x" {
		t.Fatalf("engineArgv override = %v, %v", argv, err)
	}
	argv, err = engineArgv("")
	if err != nil || len(argv) != 2 || argv[1] != "podman" {
		t.Fatalf("default engine = %v, %v (want <self> podman)", argv, err)
	}
}
