// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import "os"

// bashVersion identifies bashy as a Bash 5.3 compatible shell. It is a var,
// not a const, so a release build can stamp in a real tag/build string via
//
//	go build -ldflags "-X main.bashVersion=5.3.0(1)-bashy-v0.1.0"
//
// The "5.3.0(1)-" prefix is what the bash test suite's fixtures key on; keep
// it when overriding so compliance checks still match.
var bashVersion = "5.3.0(1)-bashy"

const (
	bashVerMajor = "5"
	bashVerMinor = "3"
	bashVerPatch = "0"
)

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
