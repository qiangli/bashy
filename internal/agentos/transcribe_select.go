package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/providerselector"
	"github.com/qiangli/yoke/pkg/spacetime"
)

// runTranscribeSelect is a thin domain-facing view over generic registered
// command declarations. The command ring still owns registration, local
// shadowing, effects and dispatch; this view knows only how a transcription
// stage maps to an open provider role.
func runTranscribeSelect(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: bashy transcribe select <acquire|decode|infer|export> [--provider COMMAND] <request.json>")
		return 0
	}
	if len(args) == 0 || !transcribeStage(args[0]) || (len(args) != 2 && len(args) != 4) {
		fmt.Fprintln(stderr, "bashy transcribe select: expected STAGE [--provider COMMAND] REQUEST.json")
		return 2
	}
	explicit, requestPath := "", args[len(args)-1]
	if len(args) == 4 {
		if args[1] != "--provider" || args[2] == "" {
			fmt.Fprintln(stderr, "bashy transcribe select: expected STAGE --provider COMMAND REQUEST.json")
			return 2
		}
		explicit = args[2]
	}
	if requestPath == "" {
		fmt.Fprintln(stderr, "bashy transcribe select: request JSON path is empty")
		return 2
	}
	data, err := os.ReadFile(requestPath)
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

	role := "transcribe." + args[0]
	var name string
	if explicit != "" {
		name, err = selectExplicitProvider(role, explicit, request, registeredCommands(), nil)
	} else {
		name, err = selectRegisteredProvider(role, request, registeredCommands(), nil)
	}
	if err != nil {
		fmt.Fprintf(stderr, "bashy transcribe select: %v\n", err)
		return 126
	}
	fmt.Fprintln(stdout, name)
	return 0
}

// selectExplicitProvider preserves the stage-facing errors while using the shared selector.
func selectExplicitProvider(role, name string, request any, records []fleet.Command, probes *spacetime.ProbeSet) (string, error) {
	for _, rec := range records {
		if rec.Name != name {
			continue
		}
		if !rec.ProvidesRole(role) {
			return "", fmt.Errorf("provider %q does not declare %q; inspect `bashy commands show %s`", name, role, name)
		}
		if verdict := rec.Applicability(probes); !verdict.Applicable {
			return "", fmt.Errorf("provider %q is unavailable on this host: %s", name, verdict.Failing)
		}
		if ok, reason := rec.AcceptsJSON(request); !ok {
			return "", fmt.Errorf("provider %q does not support this request: %s", name, reason)
		}
		opt := providerselector.Options{Role: role, Provider: name, Request: request, Yes: true, Probes: probes}
		selected, err := providerselector.Select(providerselector.Project(records, nil, opt), opt)
		if err != nil {
			return "", err
		}
		return selected.Name, nil
	}
	return "", fmt.Errorf("provider %q is not registered; register it with `bashy commands add %s`", name, name)
}

func selectRegisteredProvider(role string, request any, records []fleet.Command, probes *spacetime.ProbeSet) (string, error) {
	opt := providerselector.Options{Role: role, Request: request, Yes: true, Probes: probes}
	selected, err := providerselector.Select(providerselector.Project(records, nil, opt), opt)
	if err == nil {
		return selected.Name, nil
	}
	stage := strings.TrimPrefix(role, "transcribe.")
	if strings.HasPrefix(err.Error(), "ambiguous") {
		names := strings.TrimSuffix(strings.TrimPrefix(err.Error(), "ambiguous "+role+" providers: "), "; pass --provider NAME")
		return "", fmt.Errorf("ambiguous %s adapters: %s; pass %s NAME to select one explicitly", stage, names, transcribeProviderFlag(stage))
	}
	if strings.HasPrefix(err.Error(), "no provider declares") {
		return "", fmt.Errorf("no registered command provides %q; register one with `bashy commands add NAME --set provides.0=%s ...`", role, role)
	}
	reason := strings.TrimSuffix(strings.TrimPrefix(err.Error(), "no provider supports "+fmt.Sprintf("%q", role)+" ("), ")")
	return "", fmt.Errorf("no registered %s adapter supports this request on this host (%s); adjust its accepts/requires/os declaration or select a provider explicitly", stage, reason)
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
