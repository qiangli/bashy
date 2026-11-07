package agentos

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selfInstallDefaults fetches the source pin, then applies the defaults
// command from it; every failure is a note, never an install error.
func TestSelfInstallDefaults(t *testing.T) {
	type call struct{ args string }
	setup := func(t *testing.T, fetchErr, applyErr error, layScript bool) (string, []string) {
		t.Helper()
		home := t.TempDir()
		t.Setenv("BASHY_HOME", home)
		script := filepath.Join(home, "src", "bashy", filepath.FromSlash(selfDefaultsScript))
		var calls []string
		var out bytes.Buffer
		selfInstallDefaults("/opt/bin/bashy", &out, func(name string, arg ...string) error {
			calls = append(calls, name+" "+strings.Join(arg, " "))
			switch arg[0] {
			case "src":
				if fetchErr == nil && layScript {
					if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(script, nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return fetchErr
			default:
				return applyErr
			}
		})
		return out.String(), calls
	}

	out, calls := setup(t, nil, nil, true)
	if len(calls) != 2 || calls[0] != "/opt/bin/bashy src fetch bashy" ||
		!strings.HasPrefix(calls[1], "/opt/bin/bashy --bashsharp ") || !strings.HasSuffix(calls[1], "bashy-defaults.bsh apply") {
		t.Fatalf("calls = %q", calls)
	}
	if out != "" {
		t.Errorf("success printed a note: %q", out)
	}

	out, calls = setup(t, errors.New("offline"), nil, true)
	if len(calls) != 1 || !strings.Contains(out, "could not fetch the source pin (offline)") || !strings.Contains(out, "later: bashy src fetch bashy") {
		t.Fatalf("fetch failure: calls=%q out=%q", calls, out)
	}

	out, calls = setup(t, nil, nil, false)
	if len(calls) != 1 || !strings.Contains(out, "no defaults command") {
		t.Fatalf("older release: calls=%q out=%q", calls, out)
	}

	out, calls = setup(t, nil, errors.New("exit status 1"), true)
	if len(calls) != 2 || !strings.Contains(out, "not applied (exit status 1)") {
		t.Fatalf("apply failure: calls=%q out=%q", calls, out)
	}
}

// An upgrade or reinstall (source pin already present) skips the step.
func TestSelfInstallDefaultsSkipsWhenAlreadySetUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "src", "bashy"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	called := false
	selfInstallDefaults("/opt/bin/bashy", &out, func(string, ...string) error { called = true; return nil })
	if called {
		t.Fatal("an existing setup was fetched or applied again")
	}
	if !strings.Contains(out.String(), "already set up") || !strings.Contains(out.String(), "apply --upgrade") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestSelfInstallHasNoDefaultsFlag(t *testing.T) {
	cmd := selfInstallCmd()
	if cmd.Flags().Lookup("no-defaults") == nil {
		t.Fatal("self install has no --no-defaults flag")
	}
	if !strings.Contains(cmd.Long, "BASHY_SELF_NO_DEFAULTS=1") {
		t.Fatal("self install help does not document the opt-out")
	}
}
