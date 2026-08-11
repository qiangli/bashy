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

func TestVersionBannerDefaultsToGNUCompatibleBash(t *testing.T) {
	oldVersion, oldBuildID := bashVersion, buildID
	oldProduct, oldCompatibility := VersionProduct, VersionCompatibility
	defer func() {
		bashVersion, buildID = oldVersion, oldBuildID
		VersionProduct, VersionCompatibility = oldProduct, oldCompatibility
	}()

	bashVersion = "5.3.0(1)-bashy-dev"
	buildID = "abc1234"
	VersionProduct = "GNU bash"
	VersionCompatibility = ""
	if got, want := versionBanner(), "GNU bash, version 5.3.0(1)-bashy-dev (abc1234)"; got != want {
		t.Fatalf("versionBanner() = %q, want %q", got, want)
	}

	VersionProduct = "bashy"
	VersionCompatibility = "GNU Bash 5.3 compatible"
	if got, want := versionBanner(), "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (abc1234)"; got != want {
		t.Fatalf("bashy versionBanner() = %q, want %q", got, want)
	}
}
