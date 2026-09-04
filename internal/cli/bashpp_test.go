// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func envLookup(m map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := m[name]
		return v, ok
	}
}

func TestResolveBashPP_Precedence(t *testing.T) {
	tests := []struct {
		name       string
		sel        BashPPSelector
		wantEnable bool
		wantSource BashPPSource
	}{
		{
			name: "bash binary, no signal anywhere: off by binary default",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bashy binary, no signal anywhere: on by binary default",
			sel: BashPPSelector{
				Binary: BashPPBinaryBashy,
			},
			wantEnable: true,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bash binary, .bpp extension stays off without a selector",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
				Filename: "script.bpp",
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bashy binary records the .bpp extension tier",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBashy,
				Filename: "script.bpp",
			},
			wantEnable: true,
			wantSource: BashPPSourceExtension,
		},
		{
			name: "bash binary, plain .sh extension stays off",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
				Filename: "script.sh",
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "env BASHY_BASHPP=1 beats extension-less default",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "1"}),
			},
			wantEnable: true,
			wantSource: BashPPSourceEnv,
		},
		{
			name: "env BASHY_BASHPP=0 beats .bpp extension",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				Filename:  "script.bpp",
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "0"}),
			},
			wantEnable: false,
			wantSource: BashPPSourceEnv,
		},
		{
			name: "env BASHY_BASHPP=0 beats bashy binary default",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBashy,
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "0"}),
			},
			wantEnable: false,
			wantSource: BashPPSourceEnv,
		},
		{
			name: "env garbage value has no opinion, falls through to bash default",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				Filename:  "script.bpp",
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "yes"}),
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "env unset has no opinion, falls through to binary default",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBashy,
				LookupEnv: envLookup(map[string]string{"UNRELATED": "1"}),
			},
			wantEnable: true,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "CLI --bashpp beats env and extension",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				Args:      []string{"bash", "--bashpp", "script.sh"},
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "0"}),
			},
			wantEnable: true,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "CLI --bash++ is an exact alias of --bashpp",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--bash++", "script.sh"},
			},
			wantEnable: true,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "CLI --no-bashpp beats .bpp extension and bashy default",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBashy,
				Args:     []string{"bashy", "--no-bashpp", "script.bpp"},
				Filename: "script.bpp",
			},
			wantEnable: false,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "CLI last-flag-wins: --bashpp then --no-bashpp resolves off",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--bashpp", "--no-bashpp", "script.sh"},
			},
			wantEnable: false,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "CLI last-flag-wins: --no-bashpp then --bashpp resolves on",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--no-bashpp", "--bashpp", "script.sh"},
			},
			wantEnable: true,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "CLI scan stops at the script operand",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "script.sh", "--bashpp"},
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "CLI scan stops at -c, its operand is not scanned",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "-c", "--bashpp"},
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "CLI scan stops at --",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--", "--bashpp"},
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bashy under --posix with no signal stays on by binary default (not a certification profile)",
			sel: BashPPSelector{
				Binary: BashPPBinaryBashy,
				Posix:  true,
			},
			wantEnable: true,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bashy under --posix --bashpp is a supported combined mode",
			sel: BashPPSelector{
				Binary: BashPPBinaryBashy,
				Args:   []string{"bashy", "--posix", "--bashpp"},
				Posix:  true,
			},
			wantEnable: true,
			wantSource: BashPPSourceCLI,
		},
		{
			name: "bash under --posix with no signal stays off, no refusal",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Posix:  true,
			},
			wantEnable: false,
			wantSource: BashPPSourceBinaryDefault,
		},
		{
			name: "bash under --posix --no-bashpp explicit off, no refusal, even with .bpp",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
				Args:     []string{"bash", "--posix", "--no-bashpp"},
				Filename: "script.bpp",
				Posix:    true,
			},
			wantEnable: false,
			wantSource: BashPPSourceCLI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveBashPP(tc.sel)
			if err != nil {
				t.Fatalf("ResolveBashPP() unexpected error: %v", err)
			}
			if got.Enabled != tc.wantEnable {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tc.wantEnable)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

func TestResolveBashPP_POSIXInertnessProfile(t *testing.T) {
	tests := []struct {
		name       string
		sel        BashPPSelector
		wantPosix  bool
		wantSource BashPPSource
	}{
		{
			name: "bash --posix --bashpp selects the inert profile",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--posix", "--bashpp"},
				Posix:  true,
			},
			wantSource: BashPPSourceCLI,
		},
		{
			name: "bash --posix --bash++ selects the inert profile",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--posix", "--bash++"},
				Posix:  true,
			},
			wantSource: BashPPSourceCLI,
		},
		{
			name: "bash --posix with BASHY_BASHPP=1 selects the inert profile",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "1"}),
				Posix:     true,
			},
			wantSource: BashPPSourceEnv,
		},
		{
			name: "bash --posix with only a .bpp filename remains POSIX",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
				Filename: "script.bpp",
				Posix:    true,
			},
			wantPosix:  true,
			wantSource: BashPPSourceBinaryDefault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveBashPP(tc.sel)
			if err != nil {
				t.Fatalf("ResolveBashPP() unexpected error: %v", err)
			}
			if got.Enabled {
				t.Errorf("Enabled = true, want false for the bash POSIX inertness profile")
			}
			if got.Posix != tc.wantPosix {
				t.Errorf("Posix = %v, want %v", got.Posix, tc.wantPosix)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
		})
	}
}

func TestBashPPSource_Explicit(t *testing.T) {
	tests := []struct {
		source BashPPSource
		want   bool
	}{
		{BashPPSourceCLI, true},
		{BashPPSourceEnv, true},
		{BashPPSourceExtension, false},
		{BashPPSourceBinaryDefault, false},
	}
	for _, tc := range tests {
		if got := tc.source.Explicit(); got != tc.want {
			t.Errorf("%q.Explicit() = %v, want %v", tc.source, got, tc.want)
		}
	}
}

func TestBashPPResolution_ParserOptions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resolution BashPPResolution
		base       syntax.LangVariant
	}{
		{"bashpp", BashPPResolution{Enabled: true}, syntax.LangBash},
		{"bashpp-posix", BashPPResolution{Enabled: true, Posix: true}, syntax.LangBash},
		{"bashpp-posix-base", BashPPResolution{Enabled: true}, syntax.LangPOSIX},
		{"bash", BashPPResolution{}, syntax.LangBash},
		{"bash-posix", BashPPResolution{Posix: true}, syntax.LangBash},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := syntax.NewParser(tc.resolution.ParserOptions(tc.base)...)
			if _, err := p.Parse(strings.NewReader("echo ${x@Q}"), ""); err != nil {
				t.Fatalf("Bash grammar was not retained: %v", err)
			}
		})
	}

	// POSIX mode must remain observable when Bash++ owns the grammar.
	// In POSIX mode, single quotes in these double-quoted parameter
	// expansions are literal text rather than quote syntax.
	resolved := BashPPResolution{Enabled: true, Posix: true}
	file, err := syntax.NewParser(resolved.ParserOptions(syntax.LangBash)...).
		Parse(strings.NewReader(`echo "${a+'x'}"`), "")
	if err != nil {
		t.Fatal(err)
	}
	call := file.Stmts[0].Cmd.(*syntax.CallExpr)
	dq := call.Args[1].Parts[0].(*syntax.DblQuoted)
	pe := dq.Parts[0].(*syntax.ParamExp)
	if got := pe.Exp.Word.Lit(); got != "'x'" {
		t.Fatalf("Bash++ dropped POSIX parsing semantics: got %q, want %q", got, "'x'")
	}
}

func TestCommandLineBashPPSkipsOptionValues(t *testing.T) {
	for _, args := range [][]string{
		{"bash", "--rcfile", "foo", "--bashpp"},
		{"bash", "--init-file", "foo", "--bashpp"},
		{"bash", "-o", "errexit", "--bashpp"},
		{"bash", "-O", "extglob", "--bashpp"},
		{"bash", "-o", "errexit", "--bashpp", "--no-bashpp"},
	} {
		enabled, seen := commandLineBashPP(args)
		want := args[len(args)-1] != "--no-bashpp"
		if !seen || enabled != want {
			t.Errorf("commandLineBashPP(%q) = %v, %v; want %v, true", args, enabled, seen, want)
		}
	}
}
