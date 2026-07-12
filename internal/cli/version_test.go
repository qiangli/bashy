package cli

import "testing"

func TestBashVersionLineAppendsBuildID(t *testing.T) {
	oldVersion, oldBuildID := bashVersion, buildID
	defer func() {
		bashVersion, buildID = oldVersion, oldBuildID
	}()

	bashVersion = "5.3.0(1)-bashy-dev"
	buildID = "6e1d934-dirty"

	got := bashVersionLine()
	want := "5.3.0(1)-bashy-dev (6e1d934-dirty)"
	if got != want {
		t.Fatalf("bashVersionLine() = %q, want %q", got, want)
	}
}

func TestBashVersionLineOmitsEmptyBuildID(t *testing.T) {
	oldVersion, oldBuildID := bashVersion, buildID
	defer func() {
		bashVersion, buildID = oldVersion, oldBuildID
	}()

	bashVersion = "5.3.0(1)-bashy"
	buildID = " "

	got := bashVersionLine()
	if got != bashVersion {
		t.Fatalf("bashVersionLine() = %q, want %q", got, bashVersion)
	}
}
