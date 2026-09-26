// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

package agentos

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/cligw"
)

func TestE2ELLMFrontDoor(t *testing.T) {
	bin := bashyBinary(t)
	home := filepath.Join(t.TempDir(), "bashy-home")
	env := []string{"BASHY_HOME=" + home}

	help, stderr, code := runBashyStdEnv(bin, env, "llm", "--help")
	if code != 0 {
		t.Fatalf("bashy llm --help exited %d: %s", code, stderr)
	}
	for _, want := range []string{"serve", "pools", "env"} {
		if !strings.Contains(help, want) {
			t.Errorf("bashy llm --help missing %q:\n%s", want, help)
		}
	}

	out, stderr, code := runBashyStdEnv(bin, env, "llm", "env")
	if code != 0 {
		t.Fatalf("bashy llm env without a server exited %d: %s", code, stderr)
	}
	for _, want := range []string{
		fmt.Sprintf("OPENAI_BASE_URL=http://127.0.0.1:%d/v1", cligw.DefaultPort),
		"OPENAI_API_KEY=",
		fmt.Sprintf("ANTHROPIC_BASE_URL=http://127.0.0.1:%d/anthropic", cligw.DefaultPort),
		"export OPENAI_BASE_URL OPENAI_API_KEY ANTHROPIC_BASE_URL ANTHROPIC_API_KEY",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bashy llm env missing %q:\n%s", want, out)
		}
	}
}
