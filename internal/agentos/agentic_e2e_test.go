//go:build e2e

package agentos

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/yoke/pkg/bscript"
)

func TestAgenticE2EHelpNativeYieldAndExternalStatus(t *testing.T) {
	bin := bashyBinary(t)
	home := t.TempDir()
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"BASHY_HOME=" + home,
		"BASHY_HINTS=off",
		"YCODE_DATA_DIR=" + home,
	}

	out, _, code := runBashyStdEnv(bin, env, "agentic", "--help", "false")
	if code != 0 || !strings.Contains(out, "usage: bashy agentic") {
		t.Fatalf("help: code=%d out=%q", code, out)
	}

	out, stderr, code := runBashyStdEnv(bin, env, "agentic", "false")
	if code != weavecli.ExitInputRequired || stderr != "" {
		t.Fatalf("native failure: code=%d stderr=%q", code, stderr)
	}
	var got bscript.Yield
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("yield json: %v\n%s", err, out)
	}
	if got.Status != bscript.StatusInputRequired || got.Skill.Kind != "skill" || got.Skill.Bindings["step-run"] != "false" {
		t.Fatalf("yield = %#v", got)
	}

	if runtime.GOOS != "windows" {
		_, _, code = runBashyStdEnv(bin, env, "agentic", "/bin/sh", "-c", "exit 7")
		if code != 7 {
			t.Fatalf("external status = %d, want 7", code)
		}
	}
}
