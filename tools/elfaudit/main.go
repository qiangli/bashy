// Command elfaudit verifies the structural contract of Bashy's Cloudbox ELF.
package main

import (
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 2 && !(len(os.Args) == 3 && os.Args[1] == "--bashy-signal") {
		fmt.Fprintln(os.Stderr, "usage: elfaudit [--bashy-signal] PATH")
		os.Exit(2)
	}
	path := os.Args[len(os.Args)-1]
	var err error
	if len(os.Args) == 3 {
		err = auditBashySignal(path)
	} else {
		err = audit(path)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "elfaudit: %v\n", err)
		os.Exit(1)
	}
}

// auditBashySignal guards inherited-signal parity for shipped cmd/bashy
// artifacts. Linux needs runtime.fwdSig in a fixed-address ELF symbol table;
// Darwin needs the pre-Go C constructor. Pure-Go Darwin releases are rejected
// before publication. This check is cross-host: macOS release builders can
// audit Linux ELF files without executing them.
func auditBashySignal(path string) error {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Go build information from %s: %w", path, err)
	}
	if build.Path != "github.com/qiangli/bashy/cmd/bashy" {
		return fmt.Errorf("%s contains %q, want cmd/bashy", path, build.Path)
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	switch settings["GOOS"] {
	case "darwin":
		if settings["CGO_ENABLED"] != "1" {
			return fmt.Errorf("%s is pure-Go Darwin cmd/bashy; build natively with cgo to retain the pre-Go inherited-signal constructor", path)
		}
		f, err := macho.Open(path)
		if err != nil {
			return fmt.Errorf("open Darwin Bashy Mach-O %s: %w", path, err)
		}
		defer f.Close()
		if f.Symtab == nil {
			return fmt.Errorf("%s lacks a Mach-O symbol table for the inherited-signal constructor", path)
		}
		for _, symbol := range f.Symtab.Syms {
			if symbol.Name == "_snapshot_inherited_ignores" {
				fmt.Printf("elfaudit: PASS %s retains Darwin pre-Go inherited-signal constructor\n", path)
				return nil
			}
		}
		return fmt.Errorf("%s lacks the pre-Go inherited-signal constructor", path)
	case "linux":
		// Continue with the ELF check below.
	default:
		return nil
	}

	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open Linux Bashy ELF %s: %w", path, err)
	}
	defer f.Close()
	if f.Type != elf.ET_EXEC {
		return fmt.Errorf("%s has ELF type %s; inherited-signal repair requires ET_EXEC (disable PIE)", path, f.Type)
	}
	if f.Class != elf.ELFCLASS64 || (f.Machine != elf.EM_X86_64 && f.Machine != elf.EM_AARCH64) {
		return fmt.Errorf("%s has %s/%s; supported Linux Bashy releases require amd64 or arm64 ELF64", path, f.Class, f.Machine)
	}
	if f.SectionByType(elf.SHT_SYMTAB) == nil {
		return fmt.Errorf("%s has no ELF symbol table; do not build Linux cmd/bashy with -s or strip", path)
	}
	symbols, err := f.Symbols()
	if err != nil {
		return fmt.Errorf("read ELF symbols from %s: %w", path, err)
	}
	for _, symbol := range symbols {
		if symbol.Name == "runtime.fwdSig" && symbol.Value != 0 && symbol.Size >= 65*8 {
			fmt.Printf("elfaudit: PASS %s retains runtime.fwdSig in ET_EXEC ELF\n", path)
			return nil
		}
	}
	return fmt.Errorf("%s lacks usable runtime.fwdSig symbol; do not build Linux cmd/bashy with -s or strip", path)
}

func audit(path string) error {
	build, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Go build information from %s: %w", path, err)
	}
	settings := make(map[string]string, len(build.Settings))
	for _, setting := range build.Settings {
		settings[setting.Key] = setting.Value
	}
	for key, want := range map[string]string{
		"GOOS":        "linux",
		"GOARCH":      "amd64",
		"CGO_ENABLED": "0",
	} {
		if got := settings[key]; got != want {
			return fmt.Errorf("%s build setting is %q, want %q", key, got, want)
		}
	}
	tagged := false
	for _, tag := range strings.Split(settings["-tags"], ",") {
		if tag == "bashy_scratch" {
			tagged = true
			break
		}
	}
	if !tagged {
		return fmt.Errorf("build tags %q do not contain bashy_scratch", settings["-tags"])
	}

	f, err := elf.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_X86_64 {
		return fmt.Errorf("%s is %s/%s machine %s, want 64-bit little-endian amd64 ELF", path, f.Class, f.Data, f.Machine)
	}
	if f.Type != elf.ET_EXEC {
		return fmt.Errorf("%s has ELF type %s, want executable", path, f.Type)
	}

	for _, prog := range f.Progs {
		if prog.Type == elf.PT_INTERP {
			return fmt.Errorf("%s has an ELF INTERP program header", path)
		}
	}

	if f.SectionByType(elf.SHT_DYNAMIC) != nil {
		needed, err := f.DynString(elf.DT_NEEDED)
		if err != nil {
			return fmt.Errorf("read dynamic NEEDED entries from %s: %w", path, err)
		}
		if len(needed) != 0 {
			return fmt.Errorf("%s has ELF NEEDED entries: %v", path, needed)
		}
	}

	fmt.Printf("elfaudit: PASS %s is static linux/amd64 ELF (no INTERP or NEEDED)\n", path)
	return nil
}
