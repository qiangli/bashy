package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	hostLocalePathEnv  = "BASHY_HOST_LOCALE"
	hostLocaleNamesEnv = "BASHY_HOST_LOCALE_NAMES"
)

// corpusLocaleNames is the complete locale set named by the Bash 5.3 corpus.
// The host provider, not this list, supplies every category's data. Names that
// the provider cannot select and answer are rejected by coreutils' locale -a.
var corpusLocaleNames = []string{
	"en_US.UTF-8",
	"zh_TW.big5",
	"ja_JP.SJIS",
	"fr_FR.ISO8859-1",
	"de_DE.UTF-8",
	"zh_HK.big5hkscs",
	"ru_RU.CP1251",
}

// configureHostLocaleProvider connects the private fixture root to a real
// host POSIX locale service. It deliberately copies no locale archive or
// tables: BASHY_HOST_LOCALE names the package-owned executable, and coreutils
// delegates -a/-k calls to it while validating every category and charmap.
//
// An explicit BASHY_HOST_LOCALE wins so a caller can install a better provider
// in one step before running the harness. Otherwise the Windows runner's host
// locale executable is discovered before the fixture PATH is replaced by the
// private yoke userland. Missing service is not an infrastructure failure: the
// locale-sensitive fixtures retain their existing, truthful missing-locale
// diagnostics.
func configureHostLocaleProvider(goos string, getenv func(string) string, setenv func(string, string) error, lookPath func(string) (string, error)) (string, error) {
	if goos != "windows" {
		return "", nil
	}
	if explicit := strings.TrimSpace(getenv(hostLocalePathEnv)); explicit != "" {
		return explicit, nil
	}
	path, err := lookPath("locale")
	if err != nil {
		return "", nil
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve host locale service: %w", err)
	}
	if err := setenv(hostLocalePathEnv, path); err != nil {
		return "", err
	}
	if strings.TrimSpace(getenv(hostLocaleNamesEnv)) == "" {
		if err := setenv(hostLocaleNamesEnv, strings.Join(corpusLocaleNames, ";")); err != nil {
			return "", err
		}
	}
	return path, nil
}
