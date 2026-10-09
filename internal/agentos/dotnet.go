package agentos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/yoke/pkg/binmgr"
	"mvdan.cc/sh/v3/pathconv"
)

// dotnetSDKVersion is the .NET SDK the "dotnet" island row provisions; it is
// what `dotnet fsi` (F# .fs/.fsx source files) runs on. The SDK archive is
// pinned per platform by the SHA-512 Microsoft publishes in the release
// metadata (https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json,
// release 10.0.12, 2026-09-08), committed here so the trust anchor ships with
// bashy rather than being fetched from the origin that serves the archive.
// binmgr refuses the download if the digest does not match. Archive licence
// (MIT LICENSE.txt, permissive ThirdPartyNotices.txt): download + exec only,
// recorded in docs/fence-toolchain-licenses.md.
const dotnetSDKVersion = "10.0.401"

type dotnetAsset struct{ file, sha512 string }

var dotnetSDKAssets = map[string]dotnetAsset{
	"linux/amd64":   {"dotnet-sdk-10.0.401-linux-x64.tar.gz", "51c8b999af9e8dd9998c9edc5944e19a90788862068acd38694e098889054ce8c23d4f0c5cccfa16bf187d044562359e5ee69a9f8ad0bbe913ba90311fbce25b"},
	"linux/arm64":   {"dotnet-sdk-10.0.401-linux-arm64.tar.gz", "58ace73ced6b4360754689a686bdfb8a317f4da6cb8bb416dbc7d0ba9f47e43e3c09f5eb1f1a1cfaacbd10df9558da4882bf2a5e195d6ab56a02c1f9f76102ed"},
	"darwin/amd64":  {"dotnet-sdk-10.0.401-osx-x64.tar.gz", "33401b4a2da8554e3306db6072ea8569d9fcc608509c271e0aa4b39e7cc432da3631f14e7e1e2445d67d72550d18ce44a8bbd2382a756867ad2edab6b1c963c0"},
	"darwin/arm64":  {"dotnet-sdk-10.0.401-osx-arm64.tar.gz", "69f64eb00dc045398755c440b152225d544301a345a146a16e86a56a0c52b7c94b2c331520e976dbb821f18d31930aafbd25bb85961e3517e0665414ce0cbcff"},
	"windows/amd64": {"dotnet-sdk-10.0.401-win-x64.zip", "24b670ad3d923bfcf47df6c3b034152398b42f6dbc388e10d783aee1cfb5e5817d399fc0ae2a12cfa822a55e61d34830ccb15c50ef6efee437ab874bb7c79430"},
	"windows/arm64": {"dotnet-sdk-10.0.401-win-arm64.zip", "8272eaab6f06ad658b1976e19d88beed287a601f968b71d5c26b75d10587cf087665c599d2e136a911002f955c97f59aa8692581bbf6b8e7af5f82604c810256"},
}

// dotnetSDKTool is the binmgr tool for platform (a "goos/goarch" key). The
// archive unpacks with the `dotnet` muxer at its root.
func dotnetSDKTool(platform string) (binmgr.Tool, error) {
	a, ok := dotnetSDKAssets[platform]
	if !ok {
		return binmgr.Tool{}, fmt.Errorf("bashy provisions no .NET SDK for %s; set BASHPP_DOTNET to a dotnet you installed", platform)
	}
	entry := "dotnet"
	if strings.HasPrefix(platform, "windows/") {
		entry = "dotnet.exe"
	}
	return binmgr.Tool{
		Name: "dotnet-sdk", Version: dotnetSDKVersion,
		Assets: map[string]binmgr.Asset{platform: {
			URL:        "https://builds.dotnet.microsoft.com/dotnet/Sdk/" + dotnetSDKVersion + "/" + a.file,
			SHA512:     a.sha512,
			Tree:       true,
			Entrypoint: entry,
		}},
	}, nil
}

func provisionedDotnet(ctx context.Context) ([]string, string, error) {
	tool, err := dotnetSDKTool(binmgr.Platform())
	if err != nil {
		return nil, "", err
	}
	bin, err := binmgr.Ensure(ctx, tool)
	if err != nil {
		return nil, "", err
	}
	if cache, cerr := binmgr.CacheDir(); cerr == nil {
		for _, kv := range dotnetEnv(cache, filepath.Dir(bin), runtime.GOOS == "windows", os.Getenv) {
			name, value, _ := strings.Cut(kv, "=")
			_ = os.Setenv(name, value)
		}
	}
	return []string{bin}, "selected provisioned .NET SDK " + dotnetSDKVersion, nil
}

// dotnetEnv is the child environment the SDK needs, set on the process
// because ToolResolver's protocol carries argv but not environment: telemetry
// and first-run output off, the SDK rooted at its own tree (no machine-wide
// lookup), and the per-user state redirected under the Bashy cache so a
// read-only or absent HOME cannot fail `dotnet fsi`. A value the caller
// already set for the two state roots is kept.
func dotnetEnv(cache, sdkRoot string, windows bool, getenv func(string) string) []string {
	if windows {
		cache = pathconv.ToOSMode("", cache, true)
		sdkRoot = pathconv.ToOSMode("", sdkRoot, true)
	}
	home := filepath.Join(filepath.Dir(strings.TrimRight(cache, `\/`)), "dotnet-home")
	env := []string{
		"DOTNET_CLI_TELEMETRY_OPTOUT=1",
		"DOTNET_NOLOGO=1",
		"DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1",
		"DOTNET_MULTILEVEL_LOOKUP=0",
		"DOTNET_ROOT=" + sdkRoot,
	}
	if getenv("DOTNET_CLI_HOME") == "" {
		env = append(env, "DOTNET_CLI_HOME="+home)
	}
	if getenv("NUGET_PACKAGES") == "" {
		env = append(env, "NUGET_PACKAGES="+filepath.Join(home, "nuget-packages"))
	}
	return env
}
