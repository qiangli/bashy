// verifyhost is the release gate for a signed Bashy host asset. It uses the
// same binmgr verifier that guards service installation on a user's host.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/qiangli/yoke/pkg/binmgr"
)

type manifest struct {
	Schema     string   `json:"schema"`
	SourceSHA  string   `json:"source_sha"`
	Profile    string   `json:"profile"`
	CGOEnabled bool     `json:"cgo_enabled"`
	GoOS       string   `json:"goos"`
	GoARCH     string   `json:"goarch"`
	GoTags     []string `json:"go_tags"`
	Asset      string   `json:"asset"`
	SHA256     string   `json:"sha256"`
}

func main() {
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: verifyhost ASSET BUNDLE MANIFEST WORKFLOW_IDENTITY")
		os.Exit(2)
	}
	asset, bundlePath, manifestPath, identity := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fatal(err)
	}
	var record manifest
	if err := json.Unmarshal(data, &record); err != nil {
		fatal(err)
	}
	if record.Schema != "bashy-host-asset-v1" || record.Profile != "BASHY_ENGINES=1" || !record.CGOEnabled ||
		record.GoOS != "linux" || record.Asset != "bashy-host-linux-"+record.GoARCH ||
		(record.GoARCH != "amd64" && record.GoARCH != "arm64") ||
		len(record.SourceSHA) != 40 || len(record.GoTags) != 3 ||
		record.GoTags[0] != "bashy_engines" || record.GoTags[1] != "embed_runner" ||
		record.GoTags[2] != "bashy_host" {
		fatal(fmt.Errorf("invalid host asset manifest"))
	}
	if base := assetName(asset); base != record.Asset {
		fatal(fmt.Errorf("asset %q does not match manifest %q", base, record.Asset))
	}
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := binmgr.VerifySignedBlob(ctx, asset, bundle, record.SHA256, binmgr.Sigstore{
		Identity: identity, Issuer: binmgr.GitHubActionsIssuer,
	}); err != nil {
		fatal(err)
	}
	fmt.Printf("HOST-ASSET-BINMGR-VERIFIED %s %s\n", record.Asset, record.SHA256)
}

func assetName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "verifyhost:", err)
	os.Exit(1)
}
