package main

// TestMain duplicates the store isolation of internal/agentos's TestMain for
// the copied tests in this package: every coordination, sprint, message-board
// and foreman store is redirected to a throwaway directory, stage
// announcements are off, and an unstubbed sprint-owner launch is refused, so
// no test here can post to the real board or register a host session.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolationSentinel marks a process whose stores the top-level TestMain
// already redirected; it is inherited by re-execs of this test binary.
const isolationSentinel = "BASHY_AGENTOS_TEST_ISOLATED"

// Isolation roots installed by TestMain and the real values they shadow.
var (
	testIsolatedBase    string
	testIsolatedShimDir string
	testRealHome        string
	testRealHomeSet     bool
	testRealShimDir     string
	testRealShimSet     bool
)

// foreignReExec reports whether this test binary was launched as something
// other than `go test` (a self-exec that resolved bashySelfPath to it). Such
// a child must not re-run the suite.
func foreignReExec() bool {
	args := os.Args[1:]
	if len(args) == 0 {
		return false
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-test.") {
			return false
		}
	}
	return true
}

func TestMain(m *testing.M) {
	if foreignReExec() {
		fmt.Println(strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}
	var base string
	if os.Getenv(isolationSentinel) == "" {
		os.Setenv(isolationSentinel, "1")
		testRealShimDir, testRealShimSet = os.LookupEnv("BASHY_SHIM_DIR")
		if home, err := os.UserHomeDir(); err == nil {
			testRealHome, testRealHomeSet = home, true
		}
		b, err := os.MkdirTemp("/tmp", "bashy-yoke-test-")
		if err != nil {
			b, err = os.MkdirTemp("", "bashy-yoke-test-")
		}
		if err == nil {
			base = b
			testIsolatedBase = b
			testIsolatedShimDir = filepath.Join(b, "shims")
			_ = os.MkdirAll(testIsolatedShimDir, 0o755)
			_ = os.Setenv("BASHY_SHIM_DIR", testIsolatedShimDir)
			for _, store := range []string{
				"MB", "ROOM", "MEET", "MAILBOX", "COORD",
				"SPRINT", "FOREMAN", "RESOURCES", "HANDOFF", "TODO",
			} {
				p := filepath.Join(b, strings.ToLower(store))
				_ = os.MkdirAll(p, 0o700)
				_ = os.Setenv("BASHY_"+store+"_DIR", p)
			}
			_ = os.Setenv("BASHY_SPRINT_ANNOUNCE", "0")
			_ = os.Setenv("BASHY_COMMANDS_DIR", filepath.Join(b, "commands"))
			_ = os.Setenv("BASHY_COMMANDS_PATH", "")
			_ = os.Setenv("BASHY_SKILLS_DIR", filepath.Join(b, "skills"))
		}
	}
	runSprintOwnerCommand = func(_ context.Context, path string, args []string, _ string, _ []string) ([]byte, error) {
		return nil, fmt.Errorf("refusing unstubbed sprint-owner launch in test: %s %s", path, strings.Join(args, " "))
	}
	code := m.Run()
	if base != "" {
		_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
			if err == nil && info != nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(base)
	}
	os.Exit(code)
}
