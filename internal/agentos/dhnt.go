package agentos

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/bashy/dhnt"
)

func dispatchDhnt(args []string) int {
	if len(args) == 0 {
		printDhntUsage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "validate-pipeline":
		return dhntValidate(args[1:], true, false)
	case "canonicalize-pipeline":
		return dhntValidate(args[1:], true, true)
	case "validate-run":
		return dhntValidate(args[1:], false, false)
	case "canonicalize-run":
		return dhntValidate(args[1:], false, true)
	case "emit-run":
		return dhntEmitRun(args[1:])
	case "aggregate":
		return dhntAggregate(args[1:])
	case "help", "-h", "--help":
		printDhntUsage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "bashy dhnt: unknown command %q\n", args[0])
		printDhntUsage(os.Stderr)
		return 2
	}
}

func printDhntUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: bashy dhnt COMMAND

Commands:
  validate-pipeline [FILE|-]       strictly validate dhnt.pipeline/v1
  canonicalize-pipeline [FILE|-]   validate and print deterministic JSON
  validate-run [FILE|-]            strictly validate dhnt.run/v1
  canonicalize-run [FILE|-]        validate and print deterministic JSON
  emit-run FLAGS                   emit deterministic dhnt.run/v1 JSON
  aggregate --pipeline FILE RUN... fail-closed matrix aggregation`)
}

func dhntValidate(args []string, pipeline, canonical bool) int {
	fs := flag.NewFlagSet("bashy dhnt validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var expectNode, expectBackend, expectOS, expectArch string
	if !pipeline {
		fs.StringVar(&expectNode, "expect-node", "", "required executor node")
		fs.StringVar(&expectBackend, "expect-backend", "", "required executor backend")
		fs.StringVar(&expectOS, "expect-os", "", "required executor OS")
		fs.StringVar(&expectArch, "expect-arch", "", "required executor architecture")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "bashy dhnt: expected at most one file")
		return 2
	}
	path := "-"
	if fs.NArg() == 1 {
		path = fs.Arg(0)
	}
	data, err := readDhntFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
		return 2
	}
	if pipeline {
		value, err := dhnt.DecodePipeline(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
			return 1
		}
		if canonical {
			data, err = dhnt.MarshalPipeline(value)
			if err != nil {
				fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
				return 1
			}
			_, _ = os.Stdout.Write(data)
		}
		return 0
	}
	value, err := dhnt.DecodeRun(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
		return 1
	}
	expectations := []struct {
		name string
		got  string
		want string
	}{
		{"executor.node", value.Executor.Node, expectNode},
		{"executor.backend", value.Executor.Backend, expectBackend},
		{"executor.os", value.Executor.OS, expectOS},
		{"executor.arch", value.Executor.Arch, expectArch},
	}
	for _, expectation := range expectations {
		if expectation.want != "" && expectation.got != expectation.want {
			fmt.Fprintf(os.Stderr, "bashy dhnt: %s mismatch: got %q, want %q\n",
				expectation.name, expectation.got, expectation.want)
			return 1
		}
	}
	if canonical {
		data, err = dhnt.MarshalRun(value)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
			return 1
		}
		_, _ = os.Stdout.Write(data)
	}
	return 0
}

type artifactFlags []dhnt.Artifact

func (a *artifactFlags) String() string {
	var values []string
	for _, item := range *a {
		values = append(values, item.Name+"="+item.SHA256)
	}
	return strings.Join(values, ",")
}

type stringFlags []string

func (s *stringFlags) String() string { return strings.Join(*s, ",") }
func (s *stringFlags) Set(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	*s = append(*s, value)
	return nil
}

func (a *artifactFlags) Set(value string) error {
	name, digest, ok := strings.Cut(value, "=")
	if !ok || name == "" || digest == "" {
		return errors.New("want NAME=SHA256")
	}
	*a = append(*a, dhnt.Artifact{Name: name, SHA256: digest})
	return nil
}

func dhntEmitRun(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt emit-run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var run dhnt.Run
	var inputs, outputs artifactFlags
	var class string
	fs.StringVar(&run.Pipeline, "pipeline", "", "pipeline identity")
	fs.StringVar(&run.Task, "task", "", "task identity")
	fs.StringVar(&run.Run, "run", "", "run identity")
	fs.StringVar(&run.Source.Repository, "source-repository", "", "source repository")
	fs.StringVar(&run.Source.Commit, "source-commit", "", "source commit")
	fs.StringVar(&run.Source.SHA256, "source-sha256", "", "source tree sha256")
	fs.Var(&inputs, "input", "input NAME=SHA256 (repeatable)")
	fs.StringVar(&run.Executor.Node, "node", "", "observed executor node")
	fs.StringVar(&run.Executor.Backend, "backend", "", "observed executor backend")
	fs.StringVar(&run.Executor.OS, "os", "", "observed executor OS")
	fs.StringVar(&run.Executor.Arch, "arch", "", "observed executor architecture")
	fs.StringVar(&class, "class", "", "result class")
	var exitCode int
	fs.IntVar(&exitCode, "exit-code", 0, "process exit code")
	fs.Var(&outputs, "output", "output NAME=SHA256 (repeatable)")
	fs.StringVar(&run.StartedAt, "started-at", "", "UTC RFC3339 timestamp")
	fs.StringVar(&run.FinishedAt, "finished-at", "", "UTC RFC3339 timestamp")
	fs.StringVar(&run.TraceID, "trace-id", "", "32 lowercase hex trace ID")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "bashy dhnt emit-run: unexpected positional arguments")
		return 2
	}
	exitCodeProvided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "exit-code" {
			exitCodeProvided = true
		}
	})
	if !exitCodeProvided {
		fmt.Fprintln(os.Stderr, "bashy dhnt emit-run: --exit-code is required")
		return 2
	}
	run.Schema = dhnt.RunSchema
	run.Inputs = inputs
	run.Outputs = outputs
	run.Result.Class = dhnt.ResultClass(class)
	run.Result.ExitCode = &exitCode
	data, err := dhnt.MarshalRun(run)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt emit-run:", err)
		return 1
	}
	_, _ = os.Stdout.Write(data)
	return 0
}

func dhntAggregate(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt aggregate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var pipelinePath string
	var expectPipeline, expectRepository, expectCommit, expectSHA256 string
	var requiredOS stringFlags
	fs.StringVar(&pipelinePath, "pipeline", "", "dhnt.pipeline/v1 file")
	fs.StringVar(&expectPipeline, "expect-pipeline", "", "required pipeline identity")
	fs.StringVar(&expectRepository, "expect-source-repository", "", "required source repository")
	fs.StringVar(&expectCommit, "expect-source-commit", "", "required source commit")
	fs.StringVar(&expectSHA256, "expect-source-sha256", "", "required source sha256")
	fs.Var(&requiredOS, "require-os", "required matrix OS (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if pipelinePath == "" || fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate: --pipeline and at least one run file are required")
		return 2
	}
	pipelineJSON, err := readDhntFile(pipelinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate:", err)
		return 2
	}
	pipeline, err := dhnt.DecodePipeline(pipelineJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate: pipeline:", err)
		return 1
	}
	expectations := []struct {
		name string
		got  string
		want string
	}{
		{"pipeline", pipeline.Pipeline, expectPipeline},
		{"source.repository", pipeline.Source.Repository, expectRepository},
		{"source.commit", pipeline.Source.Commit, expectCommit},
		{"source.sha256", pipeline.Source.SHA256, expectSHA256},
	}
	for _, expectation := range expectations {
		if expectation.want != "" && expectation.got != expectation.want {
			fmt.Fprintf(os.Stderr, "bashy dhnt aggregate: %s mismatch: got %q, want %q\n",
				expectation.name, expectation.got, expectation.want)
			return 1
		}
	}
	taskLane := make(map[string]dhnt.Lane, len(pipeline.Tasks))
	for _, task := range pipeline.Tasks {
		taskLane[task.ID] = task.Lane
	}
	seenOS := map[string]bool{}
	for _, entry := range pipeline.Matrix {
		if entry.Platform.Backend == "vk-native" && taskLane[entry.Task] == dhnt.LaneNative {
			seenOS[entry.Platform.OS] = true
		}
	}
	for _, required := range requiredOS {
		if !seenOS[required] {
			fmt.Fprintf(os.Stderr, "bashy dhnt aggregate: pipeline matrix is missing required OS %q\n", required)
			return 1
		}
	}
	runJSON := make([][]byte, fs.NArg())
	for i, path := range fs.Args() {
		if path == "-" && len(fs.Args()) > 1 {
			fmt.Fprintln(os.Stderr, "bashy dhnt aggregate: stdin may only be used as the sole run file")
			return 2
		}
		runJSON[i], err = readDhntFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy dhnt aggregate: run[%d]: %v\n", i, err)
			return 2
		}
	}
	result, err := dhnt.AggregateJSON(pipelineJSON, runJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate:", err)
		return 1
	}
	data, err := dhnt.MarshalAggregate(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate:", err)
		return 1
	}
	_, _ = os.Stdout.Write(data)
	return 0
}

func readDhntFile(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", strconv.Quote(path), err)
	}
	return data, nil
}
