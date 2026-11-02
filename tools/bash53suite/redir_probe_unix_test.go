//go:build unix

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRedirectProbeRetainsRawFailures(t *testing.T) {
	for _, wrong := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "authored-failure"}[wrong], func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "candidate")
			script := `#!/bin/sh
[ ! -e phase-leftover ] || exit 91
echo phase > phase-leftover
if [ "$1" = -c ]; then
 case "$2" in
 *'${THIS_SH}'*) echo nested-raw; exit 7;;
 *'target+=3'*) printf 'target+=3\nmarker\n';;
 *'<%s>'*) printf '<2>\n';;
 *) printf 'marker\n';;
 esac
else
 echo diagnostic-raw
 exit 7
fi
`
			if wrong {
				script = string(bytes.ReplaceAll([]byte(script), []byte("<2>"), []byte("<9>")))
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			write(bin, script)
			nested := filepath.Join(dir, "usr", "bin")
			if err := os.MkdirAll(nested, 0755); err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(nested, "bash"), script)
			testsDir := filepath.Join(dir, "tests")
			if err := os.Mkdir(testsDir, 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"redir.tests", "redir.right", "redir11.sub"} {
				write(filepath.Join(testsDir, name), "authored inert input\n")
			}
			t.Setenv("BASHY_ROOT", dir)
			destination := filepath.Join(dir, "evidence")
			var log bytes.Buffer
			err := runRedirectProbe(dir, testsDir, bin, bin, destination, &log)
			if (err != nil) != wrong {
				t.Fatalf("error=%v wrong=%t log=%s", err, wrong, &log)
			}
			for _, phase := range redirectProbePhases() {
				raw, err := os.ReadFile(filepath.Join(destination, phase.Name+".raw"))
				if err != nil {
					t.Fatal(err)
				}
				if len(raw) == 0 {
					t.Fatalf("missing output for %s", phase.Name)
				}
				var status struct {
					ExitCode          int    `json:"exit_code"`
					ProcessError      string `json:"process_error"`
					AuthoredAssertion *bool  `json:"authored_assertion"`
				}
				b, err := os.ReadFile(filepath.Join(destination, phase.Name+".json"))
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(b, &status); err != nil {
					t.Fatal(err)
				}
				if phase.Expected == nil && (status.ProcessError == "" || status.ExitCode != 7 || status.AuthoredAssertion != nil) {
					t.Fatalf("diagnostic was reclassified: %s", b)
				}
			}
			if err := runRedirectProbe(dir, testsDir, bin, bin, destination, &log); err == nil {
				t.Fatal("overwrote existing evidence")
			}
			write(filepath.Join(nested, "bash"), "different candidate")
			if err := runRedirectProbe(dir, testsDir, bin, bin, filepath.Join(dir, "mismatch"), &log); err == nil {
				t.Fatal("accepted different nested binary")
			}
		})
	}
}
