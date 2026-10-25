package agentos

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// transcribe delegates to the operator's registered Bash# unit. The unit and
// its adapters use the ordinary command ring, so this front door owns no
// provider catalog, process runner, or unit receipt implementation.
const transcribePipelineCommand = "transcribe-pipeline"

type transcribeRunner func(name string, args []string) int

func runTranscribeRegistered(name string, args []string) int {
	rec, ok := registeredLookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "bashy transcribe: provider %q is unavailable; register it with `bashy commands add %s`\n", name, name)
		return 126
	}
	return runRegisteredFrontDoor(rec, args)
}

func runTranscribe(args []string, stdout, stderr io.Writer, run transcribeRunner) int {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		printTranscribeHelp(stdout)
		return 0
	}
	if len(args) > 0 && args[0] == "stage" {
		return runTranscribeStage(args[1:], stdout, stderr, run)
	}
	if len(args) > 0 && args[0] == "select" {
		return runTranscribeSelect(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("transcribe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	model := fs.String("model", "default", "local model ID")
	language := fs.String("language", "auto", "BCP 47 language or auto")
	output := fs.String("output", "", "transcript output path")
	format := fs.String("format", "txt", "txt, srt, or vtt")
	provider := fs.String("provider", "", "inference provider command")
	source := fs.String("source", "", "acquisition provider command")
	decoder := fs.String("decoder", "", "decoding provider command")
	exporter := fs.String("exporter", "", "export provider command")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) == "" {
		fmt.Fprintln(stderr, "bashy transcribe: expected exactly one file or URL; see `bashy transcribe --help`")
		return 2
	}
	if *model == "" || *language == "" || !transcribeFormat(*format) {
		fmt.Fprintln(stderr, "bashy transcribe: model and language must be nonempty; format must be txt, srt, or vtt")
		return 2
	}
	// Preserve the input as one argv element. A URL or path is never parsed as
	// shell source. The Bash# unit owns discovery and defaults beyond CPU/auto.
	forward := []string{"--model", *model, "--language", *language, "--format", strings.ToLower(*format), "--device", "cpu"}
	for _, selected := range []struct{ flag, value string }{
		{"--output", *output}, {"--provider", *provider}, {"--source", *source},
		{"--decoder", *decoder}, {"--exporter", *exporter},
	} {
		if selected.value != "" {
			forward = append(forward, selected.flag, selected.value)
		}
	}
	forward = append(forward, fs.Arg(0))
	return run(transcribePipelineCommand, forward)
}

func runTranscribeStage(args []string, stdout, stderr io.Writer, run transcribeRunner) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: bashy transcribe stage <acquire|decode|infer|export> --provider COMMAND <request.json>")
		return 0
	}
	stage := args[0]
	if !transcribeStage(stage) {
		fmt.Fprintf(stderr, "bashy transcribe stage: unknown stage %q\n", stage)
		return 2
	}
	if len(args) > 1 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Fprintln(stdout, "Usage: bashy transcribe stage <acquire|decode|infer|export> --provider COMMAND <request.json>")
		return 0
	}
	fs := flag.NewFlagSet("transcribe stage "+stage, flag.ContinueOnError)
	fs.SetOutput(stderr)
	provider := fs.String("provider", "", "registered stage provider command")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *provider == "" || fs.NArg() != 1 || fs.Arg(0) == "" {
		fmt.Fprintln(stderr, "bashy transcribe stage: expected --provider COMMAND and one request JSON path")
		return 2
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "bashy transcribe stage: read request: %v\n", err)
		return 2
	}
	var request TranscribeStageInput
	if err := json.Unmarshal(data, &request); err != nil {
		fmt.Fprintf(stderr, "bashy transcribe stage: invalid request JSON: %v\n", err)
		return 2
	}
	if request.Stage != stage {
		fmt.Fprintf(stderr, "bashy transcribe stage: request stage %q does not match %q\n", request.Stage, stage)
		return 2
	}
	if err := request.Validate(); err != nil {
		fmt.Fprintf(stderr, "bashy transcribe stage: %v\n", err)
		return 2
	}
	return run(*provider, []string{fs.Arg(0)})
}

func transcribeStage(s string) bool {
	switch s {
	case "acquire", "decode", "infer", "export":
		return true
	}
	return false
}

func transcribeFormat(s string) bool {
	switch strings.ToLower(s) {
	case "txt", "srt", "vtt":
		return true
	}
	return false
}

func printTranscribeHelp(w io.Writer) {
	fmt.Fprintln(w, `Usage: bashy transcribe [options] <file-or-url>

Examples:
  bashy transcribe --output talk.txt ./talk.wav
  bashy transcribe --format srt --language auto https://youtu.be/_j_PixI3Muo

Options:
  --model ID       Local model (default: default)
  --language TAG   Language or auto detection (default: auto)
  --output PATH    Transcript output path
  --format FORMAT  txt, srt, or vtt (default: txt)
  --provider NAME  Explicit inference provider
  --source NAME    Explicit acquisition provider
  --decoder NAME   Explicit decoder provider
  --exporter NAME  Explicit export provider

Selection: bashy transcribe select <acquire|decode|infer|export> <request.json>
Stages: bashy transcribe stage <acquire|decode|infer|export> --provider NAME <request.json>
Registered Bash# pipeline unit: transcribe-pipeline. CPU is the default device.
Setup and extensions: bashy skill show transcribe
Missing adapter or runtime? Register a provider and check its declared requirements.
Ambiguous input? Select --source, --decoder, --provider, or --exporter explicitly.
Prepare the default model before offline use: bashy transcribe-model-setup base`)
}
