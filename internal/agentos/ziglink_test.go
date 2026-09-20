package agentos

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Source-derived fixture provenance:
//   - rustc 1.98.1, commit 48a229cea, compiler/rustc_codegen_ssa/src/back/linker.rs
//     (Apache-2.0 OR MIT): Windows GNU export lists are sent to the linker as
//     list.def, through -Wl, for the observed no-comma path or -Xlinker when
//     the argument cannot be combined.
//
// The normal case is copied from Sprint 216 tour run 35506896490; boundary and
// failure cases pin the narrow adapter contract rather than accepting any .def.
// Tour run 35508410228 proves why the file must be retained: dropping it lets
// the link report success, but rustc then access-violates loading serde_derive.
func TestNormalizeWindowsGnuDefArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "rustc windows path",
			in:   []string{`-Wl,C:\Users\runneradmin\AppData\Local\bashpp\rust-target\debug\deps\rustcNPkazL\list.def`, "symbols.o"},
			want: []string{`C:/Users/runneradmin/AppData/Local/bashpp/rust-target/debug/deps/rustcNPkazL/list.def`, "symbols.o"},
		},
		{
			name: "xlinker pair",
			in:   []string{"-Xlinker", `C:\work,comma\rustc123\list.def`, "symbols.o"},
			want: []string{`C:/work,comma/rustc123/list.def`, "symbols.o"},
		},
		{
			name: "case insensitive windows basename",
			in:   []string{`-Wl,C:\work\LIST.DEF`, "symbols.o"},
			want: []string{`C:/work/LIST.DEF`, "symbols.o"},
		},
		{
			name: "cmd boundary trailing quote",
			in:   []string{`-Wl,C:\work\rustc123\list.def"`, "symbols.o"},
			want: []string{`C:/work/rustc123/list.def`, "symbols.o"},
		},
		{
			name: "cmd boundary quotes entire argument",
			in:   []string{`"-Wl,C:\work\rustc123\list.def"`, "symbols.o"},
			want: []string{`C:/work/rustc123/list.def`, "symbols.o"},
		},
		{
			name: "other def is preserved",
			in:   []string{`-Wl,C:\work\public.def`, "symbols.o"},
			want: []string{`-Wl,C:\work\public.def`, "symbols.o"},
		},
		{
			name: "dangling xlinker is preserved",
			in:   []string{"symbols.o", "-Xlinker"},
			want: []string{"symbols.o", "-Xlinker"},
		},
		{
			name: "unrelated args keep exact order and bytes",
			in:   []string{"-Wl,--nxcompat", " spaced ", "-luser32"},
			want: []string{"-Wl,--nxcompat", " spaced ", "-luser32"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeWindowsGnuDefArgs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeWindowsGnuDefArgs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeWindowsGnuResponse(t *testing.T) {
	// rustc 1.98.1 commit 48a229cea, link.rs lines 1834-1866
	// (Apache-2.0 OR MIT) writes one POSIX-escaped UTF-8 argument per line.
	in := "-Wl,C:\\\\work\\\\rustc123\\\\list.def\n-fno-use-linker-plugin\n-lgcc_eh\n-Wl,--nxcompat\n"
	want := "C:\\\\work\\\\rustc123\\\\list.def\n-Wl,--nxcompat\n-lunwind\n"
	got, changed := normalizeWindowsGnuResponse([]byte(in))
	if !changed || string(got) != want {
		t.Fatalf("normalize response = %q, %v; want %q, true", got, changed, want)
	}

	unchanged := []byte("-Wl,C:\\\\work\\\\public.def\n-luser32\n")
	got, changed = normalizeWindowsGnuResponse(unchanged)
	if changed || string(got) != string(unchanged) {
		t.Fatalf("unrelated response changed: %q, %v", got, changed)
	}
}

func TestNormalizeWindowsGnuResponseArgs(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "linker-arguments")
	data := "-Wl,C:\\\\work\\\\rustc123\\\\list.def\n-luser32\n"
	if err := os.WriteFile(source, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, cleanup, err := normalizeWindowsGnuResponseArgs([]string{"@" + source, "tail"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(got) != 2 || got[1] != "tail" || !strings.HasPrefix(got[0], "@") || got[0] == "@"+source {
		t.Fatalf("response argv = %q", got)
	}
	rewritten, err := os.ReadFile(strings.TrimPrefix(got[0], "@"))
	if err != nil {
		t.Fatal(err)
	}
	want := "C:\\\\work\\\\rustc123\\\\list.def\n-luser32\n"
	if string(rewritten) != want {
		t.Fatalf("rewritten response = %q, want %q", rewritten, want)
	}
}
