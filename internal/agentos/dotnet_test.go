package agentos

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/binmgr"
)

// F# source files ask the resolver for "dotnet"; with no row the run failed
// with "provisions no toolchain named dotnet".
func TestDotnetIsAProvisionedToolchain(t *testing.T) {
	if _, ok := islandToolchains["dotnet"]; !ok {
		t.Fatal("dotnet has no island toolchain row")
	}
	if got := strings.Join(islandToolsFor("fsharp"), ","); got != "dotnet" {
		t.Fatalf("fsharp tools = %q", got)
	}
}

// Every pin must be a SHA-512 of an official-CDN archive: binmgr refuses an
// asset with no digest, and the pin is the supply-chain anchor.
func TestDotnetSDKPinsAreVerifiedArchives(t *testing.T) {
	hex128 := regexp.MustCompile(`^[0-9a-f]{128}$`)
	for _, platform := range []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64", "windows/arm64"} {
		tool, err := dotnetSDKTool(platform)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		a := tool.Assets[platform]
		if !hex128.MatchString(a.SHA512) || a.SHA256 != "" {
			t.Errorf("%s: sha512 pin %q malformed", platform, a.SHA512)
		}
		if !strings.HasPrefix(a.URL, "https://builds.dotnet.microsoft.com/dotnet/Sdk/"+dotnetSDKVersion+"/dotnet-sdk-"+dotnetSDKVersion+"-") {
			t.Errorf("%s: url %q", platform, a.URL)
		}
		wantExt, wantEntry := ".tar.gz", "dotnet"
		if strings.HasPrefix(platform, "windows/") {
			wantExt, wantEntry = ".zip", "dotnet.exe"
		}
		if !strings.HasSuffix(a.URL, wantExt) || a.Entrypoint != wantEntry || !a.Tree {
			t.Errorf("%s: asset %+v", platform, a)
		}
	}
	if _, err := dotnetSDKTool("plan9/amd64"); err == nil || !strings.Contains(err.Error(), "BASHPP_DOTNET") {
		t.Fatalf("unsupported platform err = %v", err)
	}
}

// A cached SDK resolves with no network and the SDK environment is applied.
func TestDotnetResolverUsesCacheAndSetsEnvironment(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	for _, k := range []string{"DOTNET_CLI_HOME", "NUGET_PACKAGES", "DOTNET_ROOT", "DOTNET_NOLOGO", "DOTNET_CLI_TELEMETRY_OPTOUT", "DOTNET_SKIP_FIRST_TIME_EXPERIENCE", "DOTNET_MULTILEVEL_LOOKUP"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	tool, err := dotnetSDKTool(binmgr.Platform())
	if err != nil {
		t.Skip(err)
	}
	entry := filepath.Join(cache, "dotnet-sdk", dotnetSDKVersion, filepath.FromSlash(tool.Assets[binmgr.Platform()].Entrypoint))
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	argv, why, err := provisionedDotnet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 1 || argv[0] != entry || !strings.Contains(why, dotnetSDKVersion) {
		t.Fatalf("argv=%v why=%q", argv, why)
	}
	if os.Getenv("DOTNET_CLI_TELEMETRY_OPTOUT") != "1" || os.Getenv("DOTNET_NOLOGO") != "1" || os.Getenv("DOTNET_ROOT") != filepath.Dir(entry) {
		t.Fatalf("sdk env not applied: root=%q", os.Getenv("DOTNET_ROOT"))
	}
	if h := os.Getenv("DOTNET_CLI_HOME"); !strings.HasPrefix(h, filepath.Dir(cache)) || !strings.HasSuffix(h, "dotnet-home") {
		t.Fatalf("DOTNET_CLI_HOME = %q", h)
	}
}

func TestDotnetEnvKeepsCallerStateRoots(t *testing.T) {
	get := func(k string) string {
		if k == "DOTNET_CLI_HOME" {
			return "/mine"
		}
		return ""
	}
	env := strings.Join(dotnetEnv("/c/cache/bin", "/sdk", false, get), "\n")
	if strings.Contains(env, "DOTNET_CLI_HOME=") || !strings.Contains(env, "NUGET_PACKAGES=") {
		t.Fatalf("env = %q", env)
	}
	if runtime.GOOS == "windows" {
		win := strings.Join(dotnetEnv(`/c/Users/x/AppData/Local/bashy/bin`, `/c/sdk`, true, func(string) string { return "" }), "\n")
		if strings.Contains(win, "/c/") {
			t.Fatalf("msys path leaked: %q", win)
		}
	}
}
