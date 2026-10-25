package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/spacetime"
)

// runTranscribeSelect is a thin domain-facing view over generic registered
// command declarations. The command ring still owns registration, local
// shadowing, effects and dispatch; this view knows only how a transcription
// stage maps to an open provider role.
func runTranscribeSelect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: bashy transcribe select <acquire|decode|infer|export> <request.json>")
		return 0
	}
	if len(args) != 2 || !transcribeStage(args[0]) || args[1] == "" {
		fmt.Fprintln(stderr, "bashy transcribe select: expected a stage and one request JSON path")
		return 2
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: read request: %v\n", err)
		return 2
	}
	var contract TranscribeStageInput
	if err := json.Unmarshal(data, &contract); err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: invalid request JSON: %v\n", err)
		return 2
	}
	if contract.Stage != args[0] {
		fmt.Fprintf(stderr, "bashy transcribe select: request stage %q does not match %q\n", contract.Stage, args[0])
		return 2
	}
	if err := contract.Validate(); err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: %v\n", err)
		return 2
	}
	var request any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: invalid request JSON: %v\n", err)
		return 2
	}

	name, err := selectRegisteredProvider("transcribe."+args[0], request, registeredCommands(), nil)
	if err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: %v\n", err)
		return 126
	}
	fmt.Fprintln(stdout, name)
	return 0
}

func selectRegisteredProvider(role string, request any, records []fleet.Command, probes *spacetime.ProbeSet) (string, error) {
	if probes == nil {
		probes = spacetime.DefaultProbes(spacetime.NopCache())
	}
	var candidates, rejected []string
	declared := 0
	for _, rec := range records {
		if !rec.ProvidesRole(role) {
			continue
		}
		declared++
		if verdict := rec.Applicability(probes); !verdict.Applicable {
			rejected = append(rejected, rec.Name+": "+verdict.Failing)
			continue
		}
		if ok, reason := rec.AcceptsJSON(request); !ok {
			rejected = append(rejected, rec.Name+": "+reason)
			continue
		}
		candidates = append(candidates, rec.Name)
	}
	sort.Strings(candidates)
	sort.Strings(rejected)
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		stage := strings.TrimPrefix(role, "transcribe.")
		return "", fmt.Errorf("ambiguous %s adapters: %s; pass %s NAME to select one explicitly",
			stage, strings.Join(candidates, ", "), transcribeProviderFlag(stage))
	}
	if declared == 0 {
		return "", fmt.Errorf("no registered command provides %q; register one with `bashy commands add NAME --set provides.0=%s ...`", role, role)
	}
	return "", fmt.Errorf("no registered %s adapter supports this request on this host (%s); adjust its accepts/requires/os declaration or select a provider explicitly",
		strings.TrimPrefix(role, "transcribe."), strings.Join(rejected, "; "))
}

func transcribeProviderFlag(stage string) string {
	switch stage {
	case "acquire":
		return "--source"
	case "decode":
		return "--decoder"
	case "export":
		return "--exporter"
	default:
		return "--provider"
	}
}
