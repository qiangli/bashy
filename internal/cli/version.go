// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"os"
	"strings"
)

// bashVersion identifies bashy as a Bash 5.3 compatible shell. It is a var,
// not a const, so a release build can stamp in a real tag/build string via
//
//	go build -ldflags "-X main.bashVersion=5.3.0(1)-bashy-v0.1.0"
//
// The "5.3.0(1)-" prefix is what the bash test suite's fixtures key on; keep
// it when overriding so compliance checks still match.
var bashVersion = "5.3.0(1)-bashy"

// buildID is stamped by the Makefile from Git metadata. It is intentionally
// separate from bashVersion so the Bash-compatible version value keeps the
// exact shape parsed by fixtures and scripts. Source tarballs without .git leave
// this empty.
var buildID string

const (
	bashVerMajor = "5"
	bashVerMinor = "3"
	bashVerPatch = "0"
)

// BashyVersion returns bashy's own release version ("0.9.1") extracted
// from the stamped bashVersion suffix ("5.3.0(1)-bashy-v0.9.1"), or ""
// for an unstamped/dev build. Consumers (the skills host-version probe)
// treat "" as "omit".
func BashyVersion() string {
	_, v, ok := strings.Cut(bashVersion, "-bashy-v")
	if !ok || v == "" || v == "dev" {
		return ""
	}
	return v
}

// BuildID returns the stamped Git build identifier, usually an exact release tag
// or a short commit SHA with an optional "-dirty" suffix.
func BuildID() string {
	return strings.TrimSpace(buildID)
}

func bashVersionLine() string {
	if id := BuildID(); id != "" {
		return bashVersion + " (" + id + ")"
	}
	return bashVersion
}

// bashVersionVars returns the environment variables that identify bashy
// as a Bash 5.3 compatible shell.
func bashVersionVars() []string {
	exe, _ := os.Executable()
	if exe == "" {
		exe = "bashy"
	}
	return []string{
		"BASH=" + exe,
		"BASH_VERSION=" + bashVersion,
	}
}
