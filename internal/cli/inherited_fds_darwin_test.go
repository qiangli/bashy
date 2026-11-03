//go:build darwin

package cli

import (
	"io"
	"os"
	"os/exec"
	"slices"
	"testing"
)

func TestDarwinDiscoversHighInheritedDescriptor(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("inherited-high"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(executable, "-test.run=^TestDarwinHighDescriptorHelper$")
	c.Env = append(os.Environ(), "BASHY_TEST_HIGH_FD=1", "BASHY_INHERITED_FDS=")
	c.ExtraFiles = make([]*os.File, 298)
	c.ExtraFiles[297] = f // child descriptor 300
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
}

func TestDarwinHighDescriptorHelper(t *testing.T) {
	if os.Getenv("BASHY_TEST_HIGH_FD") != "1" {
		return
	}
	if got := startupInheritedFds(); !slices.Contains(got, 300) {
		t.Fatalf("descriptor300 absent: %v", got)
	}
	f := os.NewFile(300, "inherited-high")
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || string(got) != "inherited-high" {
		t.Fatalf("read=%q err=%v", got, err)
	}
}
