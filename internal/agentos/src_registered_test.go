package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/src"
)

// `bashy src show NAME` maps a registered command to the file that implements
// it: the script an exec entry runs, else the registry record itself.
func TestSrcShowsRegisteredSource(t *testing.T) {
	ringDir(t)
	script := filepath.Join(t.TempDir(), "rec.bsh")
	if err := os.WriteFile(script, []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRecord(t, fleet.Command{Name: "rec-test", Synopsis: "x", Exec: []string{"bashy", "--bashsharp", script}, Effects: []string{"exec"}})
	writeRecord(t, fleet.Command{Name: "inline-test", Synopsis: "y", Script: "true", Effects: []string{"pure"}})
	resetRegisteredIndex()

	info, err := src.Show("rec-test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Repo != src.RegisteredRepo || info.Package != script || info.Class != src.ClassSkill {
		t.Fatalf("exec entry: %+v, want the script %s", info, script)
	}
	info, err = src.Show("inline-test")
	if err != nil {
		t.Fatal(err)
	}
	if info.Repo != src.RegisteredRepo || !strings.HasSuffix(info.Package, "inline-test.yaml") {
		t.Fatalf("inline entry: %+v, want its registry record", info)
	}
	if _, err := src.Show("never-registered-xyz"); err == nil {
		t.Fatal("an unregistered name must stay unknown")
	}
}
