package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/assetring"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/spacetime"
)

func transcribeTestProbes(osName, arch string) *spacetime.ProbeSet {
	ps := spacetime.DefaultProbes(spacetime.NopCache())
	ps.SetStatic("os", osName)
	ps.SetStatic("arch", arch)
	return ps
}

func TestSelectRegisteredProviderMatchesOpenDeclarations(t *testing.T) {
	request := map[string]any{"input": "https://media.example.test/watch?v=one"}
	records := []fleet.Command{
		{Name: "local", Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `^[^:]+$`}}},
		{Name: "website", Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `^https://media\.example\.test/`}}},
		{Name: "wrong-host", Provides: []string{"transcribe.acquire"}, OS: []string{"linux"}},
		{Name: "missing-tool", Provides: []string{"transcribe.acquire"}, Requires: "has=s503-tool-that-is-absent"},
	}
	got, err := selectRegisteredProvider("transcribe.acquire", request, records, transcribeTestProbes("darwin", "arm64"))
	if err != nil || got != "website" {
		t.Fatalf("selection = %q, %v", got, err)
	}

	records = append(records, fleet.Command{Name: "website-override", Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `example\.test`}}})
	if _, err := selectRegisteredProvider("transcribe.acquire", request, records, transcribeTestProbes("darwin", "arm64")); err == nil || !strings.Contains(err.Error(), "ambiguous acquire adapters: website, website-override") || !strings.Contains(err.Error(), "--source") {
		t.Fatalf("ambiguity error = %v", err)
	}

	request["input"] = "ftp://unsupported.test/file"
	if _, err := selectRegisteredProvider("transcribe.acquire", request, records[:4], transcribeTestProbes("darwin", "arm64")); err == nil || !strings.Contains(err.Error(), "no registered acquire adapter supports") || !strings.Contains(err.Error(), "local:") || !strings.Contains(err.Error(), "wrong-host: os=linux") {
		t.Fatalf("unsupported error = %v", err)
	}
	if _, err := selectRegisteredProvider("transcribe.decode", request, records, transcribeTestProbes("darwin", "arm64")); err == nil || !strings.Contains(err.Error(), "no registered command provides") || !strings.Contains(err.Error(), "provides.0=transcribe.decode") {
		t.Fatalf("undeclared-role error = %v", err)
	}
}

func TestSelectExplicitProviderValidatesDeclaration(t *testing.T) {
	request := map[string]any{"input": "https://fixture.test/watch"}
	records := []fleet.Command{
		{Name: "fixture", Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `^https://fixture\.test/`}}},
		{Name: "wrong-role", Provides: []string{"transcribe.decode"}},
		{Name: "wrong-host", Provides: []string{"transcribe.acquire"}, OS: []string{"linux"}},
	}
	probes := transcribeTestProbes("darwin", "arm64")
	if got, err := selectExplicitProvider("transcribe.acquire", "fixture", request, records, probes); err != nil || got != "fixture" {
		t.Fatalf("explicit selection = %q, %v", got, err)
	}
	for _, tc := range []struct{ name, want string }{
		{"missing", "not registered"},
		{"wrong-role", "does not declare"},
		{"wrong-host", "unavailable on this host"},
	} {
		if _, err := selectExplicitProvider("transcribe.acquire", tc.name, request, records, probes); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v; want %q", tc.name, err, tc.want)
		}
	}
	request["input"] = "clip.wav"
	if _, err := selectExplicitProvider("transcribe.acquire", "fixture", request, records, probes); err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported explicit request = %v", err)
	}
}

func TestTranscribeSelectUsesCommandRingAndLocalOverride(t *testing.T) {
	local := ringDir(t)
	shared := t.TempDir()
	t.Setenv("BASHY_COMMANDS_PATH", shared)
	sharedRec := fleet.Command{
		Name: "fixture-source", Kind: fleet.KindCommand, Script: "true", Effects: []string{"read"},
		Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `^https://fixture\.test/`}},
	}
	data, err := fleet.Marshal(sharedRec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "fixture-source.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	resetRegisteredIndex()
	requestPath := filepath.Join(t.TempDir(), "request.json")
	writeRequest := func(input string) {
		t.Helper()
		data, err := json.Marshal(TranscribeStageInput{Version: TranscribeStageVersion, Stage: "acquire", Input: input})
		if err != nil {
			t.Fatalf("write request: %v", err)
		}
		if err := os.WriteFile(requestPath, data, 0o600); err != nil {
			t.Fatalf("write request: %v", err)
		}
	}
	run := func() (int, string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runTranscribe([]string{"select", "acquire", requestPath}, &stdout, &stderr, nil)
		return code, strings.TrimSpace(stdout.String()), stderr.String()
	}
	writeRequest("https://fixture.test/watch")
	if code, out, errout := run(); code != 0 || out != "fixture-source" {
		t.Fatalf("shared selection: code=%d out=%q err=%q", code, out, errout)
	}
	if rec, ok := registeredLookup("fixture-source"); !ok || rec.Ring != assetring.RingShared {
		t.Fatalf("shared record = %+v, %v", rec, ok)
	}

	// The ordinary writable command ring shadows the shared declaration. No
	// selector-specific override store or core rebuild is involved.
	if err := registeredCatalog().SaveCommand(fleet.Command{
		Name: "fixture-source", Script: "true", Effects: []string{"read"},
		Provides: []string{"transcribe.acquire"}, Accepts: []fleet.CommandMatch{{Path: "/input", Pattern: `^[^:]+$`}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local, "fixture-source.yaml")); err != nil {
		t.Fatal(err)
	}
	resetRegisteredIndex()
	if code, _, errout := run(); code != 126 || !strings.Contains(errout, "does not match") {
		t.Fatalf("overridden URL selection: code=%d err=%q", code, errout)
	}
	writeRequest("clip with spaces.wav")
	if code, out, errout := run(); code != 0 || out != "fixture-source" {
		t.Fatalf("local override selection: code=%d out=%q err=%q", code, out, errout)
	}
	if rec, ok := registeredLookup("fixture-source"); !ok || rec.Ring != assetring.RingLocal {
		t.Fatalf("local record = %+v, %v", rec, ok)
	}
	var stdout, stderr bytes.Buffer
	if code := runTranscribe([]string{"select", "acquire", "--provider", "fixture-source", requestPath}, &stdout, &stderr, nil); code != 0 || strings.TrimSpace(stdout.String()) != "fixture-source" {
		t.Fatalf("explicit local selection: code=%d out=%q err=%q", code, stdout.String(), stderr.String())
	}
}
