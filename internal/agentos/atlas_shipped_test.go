package agentos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

// The block title prints an origin's value once: "bash — bash builtin" keeps
// its prefix (the label merely begins with the same word), while the yoke and
// registered labels already open with "<origin> — " and are printed alone.
func TestOriginBlockTitle(t *testing.T) {
	for _, tc := range []struct{ origin, want string }{
		{atlas.OriginBash, "bash — bash builtin"},
		{atlas.OriginGNU, "gnu — GNU coreutils"},
		{atlas.OriginBashy, atlas.OriginLabel(atlas.OriginBashy)},
		{atlas.OriginRegistered, "registered — added with bashy commands add"},
	} {
		if got := originBlockTitle(tc.origin); got != tc.want {
			t.Errorf("originBlockTitle(%q) = %q, want %q", tc.origin, got, tc.want)
		}
	}
}

func TestShippedRegisteredViews(t *testing.T) {
	records := []atlasRecord{
		{Name: "cat", Origin: atlas.OriginGNU, Posix: true},
		{Name: "cd", Origin: atlas.OriginBash, Posix: true},
		{Name: "weave", Origin: atlas.OriginBashy},
		{Name: "gl", Origin: atlas.OriginRegistered},
		{Name: "hid", Origin: atlas.OriginRegistered, Hidden: true},
	}
	shipped := filterShipped(records, true)
	if len(shipped) != 3 {
		t.Fatalf("shipped = %d records, want 3", len(shipped))
	}
	registered := filterShipped(records, false)
	if len(registered) != 2 {
		t.Fatalf("registered = %d records, want 2", len(registered))
	}
	var out bytes.Buffer
	printAtlasShipped(&out, shipped, records)
	for _, want := range []string{
		"shipped — every command bashy ships, by who defined it (3; * = POSIX-required, 2):",
		"  bash — bash builtin (1):",
		"    cd*",
		"  gnu — GNU coreutils (1):",
		"  registered — yours, not shipped (2): `bashy commands --view registered`",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("shipped view missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "gl") {
		t.Errorf("shipped view carries a registered name:\n%s", out.String())
	}
	out.Reset()
	printAtlasRegistered(&out, registered)
	if !strings.Contains(out.String(), "(2; ~ = hidden)") || !strings.Contains(out.String(), "gl hid~") {
		t.Errorf("registered view:\n%s", out.String())
	}
	out.Reset()
	printAtlasRegistered(&out, nil)
	if !strings.HasPrefix(out.String(), "registered — yours (0): no registered commands") {
		t.Errorf("empty registered view:\n%s", out.String())
	}
}
