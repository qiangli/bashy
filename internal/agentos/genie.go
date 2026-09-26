package agentos

// Sprint: #290; Story: #933; Story-ID: 6a0507862385
//
// bashy genie — the front door to genie, the local-model SWE agent (ycode's
// examples/genie). Rod-thin: this file only finds or builds the genie bundle
// and hands the task to it. Everything else — the host-aware model pick, the
// run's own model server on a free port, the one-time model pull, the ycode
// run — is the bundle's own `solve` recipe (dag.md), editable like any fish.
//
//	bashy genie TASK...        solve TASK in the current git repository
//	bashy genie build [--from DIR]
//	bashy genie doctor [--json]
//	bashy genie pull           (no released bundle yet: says how to build)

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const genieUsage = `usage: bashy genie "TASK"            solve TASK in the current git repository
       bashy genie build [--from DIR]  package genie from its source (ycode/examples/genie)
       bashy genie doctor [--json]     bundle, bashy, ycode, host facts and the model pick
       bashy genie pull                fetch a released bundle (not published yet)

Environment: GENIE_BAR (bundle path), GENIE_SOURCE (source directory),
GENIE_MODEL_ID (model override), YCODE_BIN (ycode binary); the bundle's
dag.md documents the rest.
`

func dispatchGenie(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, genieUsage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, genieUsage)
		return 0
	case "build":
		return genieBuild(args[1:], os.Stderr)
	case "doctor":
		return genieDoctor(args[1:], os.Stdout, os.Stderr)
	case "pull":
		fmt.Fprintln(os.Stderr, "bashy genie pull: no released genie bundle is published yet; build one from source with `bashy genie build` (GENIE_SOURCE or --from pointing at ycode/examples/genie)")
		return 2
	}
	bundle, err := genieBundle()
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy genie:", err)
		return 2
	}
	return runSelf(append([]string{"run", "--target", "solve", bundle}, args...), os.Stdin, os.Stdout, os.Stderr)
}

// genieHome is where the installed bundle lives: $BASHY_HOME/genie, else
// ~/.bashy/genie — the same root bashy's bundle cache uses (bar.go).
func genieHome() (string, error) {
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".bashy")
	}
	return filepath.Join(home, "genie"), nil
}

// genieBundle resolves the bundle to run: $GENIE_BAR, else the installed one.
func genieBundle() (string, error) {
	if path := strings.TrimSpace(os.Getenv("GENIE_BAR")); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("GENIE_BAR: %w", err)
		}
		return path, nil
	}
	home, err := genieHome()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, "genie.bar")
	if _, err := os.Stat(path); err != nil {
		return "", errors.New("no genie bundle installed; run `bashy genie build` (from a ycode checkout, or with GENIE_SOURCE / --from pointing at ycode/examples/genie)")
	}
	return path, nil
}

// genieSource finds the genie source directory: --from, $GENIE_SOURCE, or the
// nearest ancestor of the working directory holding examples/genie or
// ycode/examples/genie.
func genieSource(from string) (string, error) {
	if from == "" {
		from = strings.TrimSpace(os.Getenv("GENIE_SOURCE"))
	}
	if from != "" {
		if !isGenieSource(from) {
			return "", fmt.Errorf("%s is not a genie source directory (no dag.md with a solve target)", from)
		}
		return filepath.Abs(from)
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		for _, rel := range []string{".", "examples/genie", "ycode/examples/genie"} {
			candidate := filepath.Join(dir, rel)
			if isGenieSource(candidate) {
				return candidate, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no genie source found above the working directory; pass --from DIR or set GENIE_SOURCE")
		}
		dir = parent
	}
}

func isGenieSource(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "dag.md"))
	return err == nil && strings.Contains(string(data), "\n### solve\n") && strings.Contains(string(data), "genie")
}

func genieBuild(args []string, stderr io.Writer) int {
	from := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--from" && i+1 < len(args):
			from = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--from="):
			from = strings.TrimPrefix(args[i], "--from=")
		default:
			fmt.Fprintf(stderr, "bashy genie build: unknown argument %q\n", args[i])
			return 2
		}
	}
	source, err := genieSource(from)
	if err != nil {
		fmt.Fprintln(stderr, "bashy genie build:", err)
		return 2
	}
	cmd := exec.Command(bashySelfPath(), "dag", "-f", "dag.md", "package")
	cmd.Dir = source
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(stderr, "bashy genie build: packaging failed:", err)
		return 1
	}
	home, err := genieHome()
	if err == nil {
		err = os.MkdirAll(home, 0o755)
	}
	if err != nil {
		fmt.Fprintln(stderr, "bashy genie build:", err)
		return 1
	}
	data, err := os.ReadFile(filepath.Join(source, "dist", "genie.bar"))
	if err != nil {
		fmt.Fprintln(stderr, "bashy genie build:", err)
		return 1
	}
	target := filepath.Join(home, "genie.bar")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		fmt.Fprintln(stderr, "bashy genie build:", err)
		return 1
	}
	if err := os.Rename(tmp, target); err != nil {
		fmt.Fprintln(stderr, "bashy genie build:", err)
		return 1
	}
	sum := sha256.Sum256(data)
	fmt.Fprintf(stderr, "bashy genie: installed %s (sha256 %s) from %s\n", target, hex.EncodeToString(sum[:]), source)
	return 0
}

type genieDoctorReport struct {
	SchemaVersion string          `json:"schema_version"`
	Bundle        string          `json:"bundle,omitempty"`
	BundleSHA256  string          `json:"bundle_sha256,omitempty"`
	BundleError   string          `json:"bundle_error,omitempty"`
	Bashy         string          `json:"bashy"`
	Ycode         string          `json:"ycode,omitempty"`
	YcodeVersion  string          `json:"ycode_version,omitempty"`
	YcodeError    string          `json:"ycode_error,omitempty"`
	ModelChoice   json.RawMessage `json:"model_choice,omitempty"`
	ModelError    string          `json:"model_error,omitempty"`
}

func genieDoctor(args []string, stdout, stderr io.Writer) int {
	asJSON := len(args) == 1 && args[0] == "--json"
	if len(args) > 1 || (len(args) == 1 && !asJSON) {
		fmt.Fprintln(stderr, "usage: bashy genie doctor [--json]")
		return 2
	}
	report := genieDoctorReport{SchemaVersion: "bashy-genie-doctor-v1", Bashy: bashySelfPath()}
	if bundle, err := genieBundle(); err != nil {
		report.BundleError = err.Error()
	} else {
		report.Bundle = bundle
		if data, err := os.ReadFile(bundle); err == nil {
			sum := sha256.Sum256(data)
			report.BundleSHA256 = hex.EncodeToString(sum[:])
		}
		// The pick is the bundle's own recipe; doctor shows its answer for
		// this host (facts included) without starting a model server.
		var out strings.Builder
		cmd := exec.Command(bashySelfPath(), "run", "--target", "pick-model", bundle)
		cmd.Stdout, cmd.Stderr = io.Discard, &out
		if err := cmd.Run(); err != nil {
			report.ModelError = strings.TrimSpace(lastLine(out.String()))
		} else if root, err := materializeBar(bundle); err != nil {
			report.ModelError = err.Error()
		} else if choice, err := os.ReadFile(filepath.Join(root, "dist", "model-choice.json")); err != nil {
			report.ModelError = "pick-model wrote no model-choice.json: " + err.Error()
		} else {
			report.ModelChoice = json.RawMessage(choice)
		}
	}
	ycode := strings.TrimSpace(os.Getenv("YCODE_BIN"))
	if ycode == "" {
		ycode, _ = exec.LookPath("ycode")
	}
	if ycode == "" {
		report.YcodeError = "ycode not found (set YCODE_BIN or put ycode on PATH)"
	} else {
		report.Ycode = ycode
		if out, err := exec.Command(ycode, "version").Output(); err == nil {
			report.YcodeVersion = strings.TrimSpace(lastLine(string(out)))
		} else {
			report.YcodeError = "ycode version: " + err.Error()
		}
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		fmt.Fprintf(stdout, "bundle:  %s%s\n", or(report.Bundle, report.BundleError), suffix(" sha256 ", report.BundleSHA256))
		fmt.Fprintf(stdout, "bashy:   %s\n", report.Bashy)
		fmt.Fprintf(stdout, "ycode:   %s%s\n", or(report.Ycode, report.YcodeError), suffix(" ", report.YcodeVersion))
		if report.ModelChoice != nil {
			var choice struct {
				Model  string `json:"model"`
				Tier   string `json:"tier"`
				Reason string `json:"reason"`
				Memory struct {
					Kind     string  `json:"kind"`
					UsableGB float64 `json:"usable_gb"`
				} `json:"memory"`
			}
			_ = json.Unmarshal(report.ModelChoice, &choice)
			fmt.Fprintf(stdout, "model:   %s (tier %s, %.1f GB usable %s): %s\n", choice.Model, choice.Tier, choice.Memory.UsableGB, choice.Memory.Kind, choice.Reason)
		} else if report.ModelError != "" {
			fmt.Fprintf(stdout, "model:   %s\n", report.ModelError)
		}
	}
	if report.BundleError != "" || report.YcodeError != "" || report.ModelError != "" {
		return 1
	}
	return 0
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func suffix(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}
