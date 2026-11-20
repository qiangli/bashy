package agentos

import (
	"strings"
	"testing"
)

// The test binary is a lean build, but the genie tests exercise the recipe as
// if a local model could run; only the refusal test below asks for "lean".
func init() { genieOllamaBuild = func() string { return "embedded" } }

// A lean bashy has no local model engine: a genie run that would serve a local
// model is refused up front with the way out, instead of pulling a model and
// failing inside the recipe. Hosted models and engine builds are never refused.
func TestGenieLocalModelRefusalOnLeanBuild(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BASHY_HOME", home+"/.bashy")
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XDG_CACHE_HOME", home+"/.cache")
	t.Setenv("BASHY_CLOUDBOX_URL", "")
	t.Setenv("GENIE_MODEL_ID", "")
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	build := "lean"
	old := genieOllamaBuild
	genieOllamaBuild = func() string { return build }
	t.Cleanup(func() { genieOllamaBuild = old })

	msg := genieLocalModelRefusal("bashy ycode", "")
	for _, want := range []string{"bashy ycode: this bashy is the lean build", "none was chosen", "bashy ycode -m MODEL", "bashy dag build-host"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("auto-pick refusal lacks %q:\n%s", want, msg)
		}
	}
	if msg := genieLocalModelRefusal("bashy genie", "gpt-oss:20b"); !strings.Contains(msg, "the local model gpt-oss:20b") {
		t.Fatalf("local tag refusal:\n%s", msg)
	}
	t.Setenv("GENIE_MODEL_ID", "gpt-oss:20b")
	if msg := genieLocalModelRefusal("bashy genie", ""); !strings.Contains(msg, "the local model gpt-oss:20b") {
		t.Fatalf("GENIE_MODEL_ID refusal:\n%s", msg)
	}
	t.Setenv("GENIE_EXTERNAL_MODEL", "hosted")
	if msg := genieLocalModelRefusal("bashy genie", ""); msg != "" {
		t.Fatalf("external model refused:\n%s", msg)
	}
	t.Setenv("GENIE_EXTERNAL_MODEL", "")
	build = "embedded"
	if msg := genieLocalModelRefusal("bashy ycode", ""); msg != "" {
		t.Fatalf("engines build refused:\n%s", msg)
	}
}
