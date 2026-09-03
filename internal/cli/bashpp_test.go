// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"errors"
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
			name: "bash binary, .bpp extension: on",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
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
			name: "env garbage value has no opinion, falls through to extension",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				Filename:  "script.bpp",
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "yes"}),
			},
			wantEnable: true,
			wantSource: BashPPSourceExtension,
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

func TestResolveBashPP_CertificationRefusal(t *testing.T) {
	tests := []struct {
		name string
		sel  BashPPSelector
	}{
		{
			name: "bash --posix --bashpp is refused",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--posix", "--bashpp"},
				Posix:  true,
			},
		},
		{
			name: "bash --posix --bash++ is refused (alias)",
			sel: BashPPSelector{
				Binary: BashPPBinaryBash,
				Args:   []string{"bash", "--posix", "--bash++"},
				Posix:  true,
			},
		},
		{
			name: "bash --posix with BASHY_BASHPP=1 is refused",
			sel: BashPPSelector{
				Binary:    BashPPBinaryBash,
				LookupEnv: envLookup(map[string]string{"BASHY_BASHPP": "1"}),
				Posix:     true,
			},
		},
		{
			name: "bash --posix with a .bpp script is refused",
			sel: BashPPSelector{
				Binary:   BashPPBinaryBash,
				Filename: "script.bpp",
				Posix:    true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveBashPP(tc.sel)
			if err == nil {
				t.Fatalf("ResolveBashPP() = %+v, want a certification refusal error", got)
			}
			var certErr *BashPPCertificationError
			if !errors.As(err, &certErr) {
				t.Fatalf("error = %v (%T), want *BashPPCertificationError", err, err)
			}
			if got != (BashPPResolution{}) {
				t.Errorf("resolution on refusal = %+v, want zero value", got)
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

func TestBashPPResolution_LangVariant(t *testing.T) {
	enabled := BashPPResolution{Enabled: true, Source: BashPPSourceCLI}
	if got := enabled.LangVariant(syntax.LangBash); got != syntax.LangBashPP {
		t.Errorf("enabled.LangVariant(LangBash) = %v, want LangBashPP", got)
	}
	if got := enabled.LangVariant(syntax.LangPOSIX); got != syntax.LangBashPP {
		t.Errorf("enabled.LangVariant(LangPOSIX) = %v, want LangBashPP", got)
	}

	disabled := BashPPResolution{Enabled: false, Source: BashPPSourceBinaryDefault}
	if got := disabled.LangVariant(syntax.LangBash); got != syntax.LangBash {
		t.Errorf("disabled.LangVariant(LangBash) = %v, want LangBash unchanged", got)
	}
	if got := disabled.LangVariant(syntax.LangPOSIX); got != syntax.LangPOSIX {
		t.Errorf("disabled.LangVariant(LangPOSIX) = %v, want LangPOSIX unchanged", got)
	}
}

func TestBashPPCertificationError_Message(t *testing.T) {
	err := &BashPPCertificationError{Source: BashPPSourceCLI}
	msg := err.Error()
	if msg == "" {
		t.Fatal("Error() returned empty string")
	}
	// The message must name the actionable escapes, not just the refusal.
	for _, want := range []string{"--posix", "bash binary", "--no-bashpp"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, want it to mention %q", msg, want)
		}
	}
}
