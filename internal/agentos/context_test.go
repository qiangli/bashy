// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectContextIncludesBashyPathAndCapabilities(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	report := collectContext()
	if report.SchemaVersion != contextSchemaVersion {
		t.Fatalf("schema = %q", report.SchemaVersion)
	}
	if report.BashyPath == "" || !filepath.IsAbs(report.BashyPath) {
		t.Fatalf("bashy path should be absolute, got %q", report.BashyPath)
	}
	if !report.Mode.Agentic {
		t.Fatalf("agentic mode not detected: %#v", report.Mode)
	}
	if !report.Capabilities.DryRun || !report.Capabilities.CheckAgentJSON || !report.Capabilities.CommandFeatures {
		t.Fatalf("missing expected capabilities: %#v", report.Capabilities)
	}
	// The recommended commands must embed the absolute bashy path (cross-platform:
	// a Windows path is C:\… not /…, so check containment of BashyPath, not a
	// leading slash).
	if report.BashyPath == "" || !filepath.IsAbs(report.BashyPath) {
		t.Fatalf("bashy_path should be absolute, got %q", report.BashyPath)
	}
	var sawPathInCommand bool
	for _, cmd := range report.RecommendedCommands {
		if strings.Contains(cmd.Command, report.BashyPath) {
			sawPathInCommand = true
		}
	}
	if !sawPathInCommand {
		t.Fatalf("recommended commands should include the absolute bashy path %q: %#v", report.BashyPath, report.RecommendedCommands)
	}
}

func TestContextReportJSON(t *testing.T) {
	b, err := json.Marshal(collectContext())
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SchemaVersion string `json:"schema_version"`
		BashyPath     string `json:"bashy_path"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("context JSON invalid: %v\n%s", err, b)
	}
	if payload.SchemaVersion != contextSchemaVersion || payload.BashyPath == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
