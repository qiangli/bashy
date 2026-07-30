package agentos

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	case "verify-native-result":
		return dhntVerifyNativeResult(args[1:])
	case "wrap-native-result":
		return dhntWrapNativeResult(args[1:])
	case "aggregate":
		return dhntAggregate(args[1:])
	case "lower-argo":
		return dhntLowerArgo(args[1:])
	case "plan-dks":
		return dhntPlanDKS(args[1:])
	case "run-task":
		return dhntRunTask(args[1:])
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
  validate-pipeline [FILE|-]       strictly validate dhnt.pipeline/v1 or v2
  canonicalize-pipeline [FILE|-]   validate and print deterministic JSON
  validate-run [FILE|-]            strictly validate dhnt.run/v1 or v2
  canonicalize-run [FILE|-]        validate and print deterministic JSON
  emit-run FLAGS                   emit deterministic dhnt.run/v1 JSON
  verify-native-result FLAGS [FILE|-]
                                   verify one bounded vk-native result artifact
  wrap-native-result --run FILE --artifact FILE
                                   emit one bounded canonical result marker
  aggregate --pipeline FILE RUN... fail-closed matrix aggregation
  lower-argo --binding FILE [PIPELINE|-]
                                   compile a strict DKS Argo Workflow
  plan-dks --inventory FILE [PIPELINE|-]
                                   plan capacity/topology/chunk placement
  run-task --workspace DIR --spec-base64 DATA -- ARGV...
                                   execute one trusted runner task`)
}

func dhntWrapNativeResult(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt wrap-native-result", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var runPath, artifactPath string
	fs.StringVar(&runPath, "run", "", "canonical passing dhnt.run/v2 file")
	fs.StringVar(&artifactPath, "artifact", "", "small regular file bound by the run")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || runPath == "" || artifactPath == "" {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result: --run and --artifact are required")
		return 2
	}
	runData, err := readDhntFile(runPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 2
	}
	run, err := dhnt.DecodeRun(runData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	if run.Schema != dhnt.RunSchemaV2 || len(run.Outputs) != 1 {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result: run must be dhnt.run/v2 with exactly one output")
		return 1
	}
	linkInfo, err := os.Lstat(artifactPath)
	if err != nil || linkInfo.Mode()&os.ModeSymlink != 0 {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result: artifact must be a regular non-symlink file")
		return 1
	}
	artifactFile, err := os.Open(artifactPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	defer artifactFile.Close()
	info, err := artifactFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result: artifact must be a regular non-symlink file")
		return 1
	}
	if info.Size() > dhnt.MaxNativeResultArtifactBytes {
		fmt.Fprintf(os.Stderr, "bashy dhnt wrap-native-result: artifact exceeds %d bytes\n",
			dhnt.MaxNativeResultArtifactBytes)
		return 1
	}
	artifactData, err := io.ReadAll(io.LimitReader(
		artifactFile, dhnt.MaxNativeResultArtifactBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	if len(artifactData) > dhnt.MaxNativeResultArtifactBytes {
		fmt.Fprintf(os.Stderr, "bashy dhnt wrap-native-result: artifact exceeds %d bytes\n",
			dhnt.MaxNativeResultArtifactBytes)
		return 1
	}
	result, err := dhnt.NewNativeResult(run, run.Outputs[0], artifactData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	data, err := dhnt.MarshalNativeResult(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	if _, err := fmt.Fprint(os.Stdout, dhnt.NativeResultMarker); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt wrap-native-result:", err)
		return 1
	}
	return 0
}

func dhntVerifyNativeResult(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt verify-native-result", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var name, kind, digest, node, backend, goos, arch, output string
	fs.StringVar(&name, "expect-name", "", "required artifact name")
	fs.StringVar(&kind, "expect-kind", "", "required artifact kind (file)")
	fs.StringVar(&digest, "expect-sha256", "", "required artifact SHA-256")
	fs.StringVar(&node, "expect-node", "", "live Pod executor node")
	fs.StringVar(&backend, "expect-backend", "", "live Node backend")
	fs.StringVar(&goos, "expect-os", "", "live Node OS")
	fs.StringVar(&arch, "expect-arch", "", "live Node architecture")
	fs.StringVar(&output, "artifact-output", "", "exclusive destination for verified bytes")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 || name == "" || kind == "" || digest == "" ||
		node == "" || backend == "" || goos == "" || arch == "" || output == "" {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result: all --expect-* flags and --artifact-output are required")
		return 2
	}
	input := "-"
	if fs.NArg() == 1 {
		input = fs.Arg(0)
	}
	var reader io.Reader = os.Stdin
	var file *os.File
	var err error
	if input != "-" {
		file, err = os.Open(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
			return 2
		}
		defer file.Close()
		reader = file
	}
	message, err := io.ReadAll(io.LimitReader(reader, dhnt.MaxNativeResultMessageBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
		return 2
	}
	digestAlgorithm := dhnt.DigestSHA256FileV1
	if kind != string(dhnt.ArtifactFile) {
		digestAlgorithm = dhnt.DigestSHA256TreeV1
	}
	payload, run, err := dhnt.VerifyNativeResultMessage(message, dhnt.NativeResultExpectation{
		Artifact: dhnt.Artifact{
			Name: name, Kind: dhnt.ArtifactKind(kind),
			DigestAlgorithm: digestAlgorithm, SHA256: digest,
		},
		Executor: dhnt.Executor{
			Node: node, Backend: backend, OS: goos, Arch: arch,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
		return 1
	}
	if err := publishExclusiveFile(output, payload); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
		return 1
	}
	canonical, err := dhnt.MarshalRun(run)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
		return 1
	}
	if _, err := os.Stdout.Write(canonical); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt verify-native-result:", err)
		return 1
	}
	return 0
}

func publishExclusiveFile(target string, content []byte) error {
	parent := filepath.Dir(target)
	staged, err := os.CreateTemp(parent, "."+filepath.Base(target)+".")
	if err != nil {
		return err
	}
	stagedName := staged.Name()
	defer os.Remove(stagedName)
	if err := staged.Chmod(0o600); err != nil {
		staged.Close()
		return err
	}
	if _, err := staged.Write(content); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	if err := os.Link(stagedName, target); err != nil {
		return fmt.Errorf("publish verified artifact without overwrite: %w", err)
	}
	return nil
}

func dhntRunTask(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt run-task", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var workspace, encodedSpec string
	fs.StringVar(&workspace, "workspace", "", "opened workspace boundary")
	fs.StringVar(&encodedSpec, "spec-base64", "", "canonical dhnt.runner-spec/v1")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	argv := fs.Args()
	if workspace == "" || encodedSpec == "" || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "bashy dhnt run-task: --workspace, --spec-base64, and argv after -- are required")
		return 2
	}
	specJSON, err := base64.StdEncoding.Strict().DecodeString(encodedSpec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt run-task: spec:", err)
		return 2
	}
	spec, err := dhnt.DecodeRunnerSpec(specJSON)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt run-task: spec:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	run, err := dhnt.ExecuteTask(ctx, workspace, spec, argv, dhnt.RunnerMetadata{
		Node:   os.Getenv("DHNT_EXECUTOR_NODE"),
		PodUID: os.Getenv("DHNT_POD_UID"),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt run-task:", err)
		return 1
	}
	if run.Result.Class == dhnt.ResultPass {
		return 0
	}
	if run.Result.ExitCode != nil && *run.Result.ExitCode > 0 && *run.Result.ExitCode <= 255 {
		return *run.Result.ExitCode
	}
	return 1
}

func dhntLowerArgo(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt lower-argo", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var bindingPath string
	fs.StringVar(&bindingPath, "binding", "", "dhnt.argo-binding/v1 or v2 file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if bindingPath == "" || fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo: --binding and at most one pipeline file are required")
		return 2
	}
	pipelinePath := "-"
	if fs.NArg() == 1 {
		pipelinePath = fs.Arg(0)
	}
	pipelineData, err := readDhntFile(pipelinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo:", err)
		return 2
	}
	bindingData, err := readDhntFile(bindingPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo:", err)
		return 2
	}
	pipeline, err := dhnt.DecodePipeline(pipelineData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo: pipeline:", err)
		return 1
	}
	binding, err := dhnt.DecodeArgoBinding(bindingData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo: binding:", err)
		return 1
	}
	output, err := dhnt.LowerArgo(pipeline, binding)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo:", err)
		return 1
	}
	if _, err := os.Stdout.Write(output); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt lower-argo:", err)
		return 1
	}
	return 0
}

func dhntPlanDKS(args []string) int {
	fs := flag.NewFlagSet("bashy dhnt plan-dks", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var inventoryPath, nowText string
	var maxFactAge time.Duration
	fs.StringVar(&inventoryPath, "inventory", "", "dhnt.host-facts/v1 file")
	fs.DurationVar(&maxFactAge, "max-fact-age", 10*time.Minute, "maximum accepted age of observed facts")
	fs.StringVar(&nowText, "now", "", "RFC3339 planning time (default current UTC time)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if inventoryPath == "" || fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks: --inventory and at most one pipeline file are required")
		return 2
	}
	if maxFactAge <= 0 {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks: --max-fact-age must be positive")
		return 2
	}
	now := time.Now().UTC()
	if nowText != "" {
		parsed, err := time.Parse(time.RFC3339, nowText)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks: --now must be RFC3339:", err)
			return 2
		}
		now = parsed.UTC()
	}
	pipelinePath := "-"
	if fs.NArg() == 1 {
		pipelinePath = fs.Arg(0)
	}
	pipelineData, err := readDhntFile(pipelinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks:", err)
		return 2
	}
	inventoryData, err := readDhntFile(inventoryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks:", err)
		return 2
	}
	pipeline, err := dhnt.DecodePipeline(pipelineData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks: pipeline:", err)
		return 1
	}
	inventory, err := dhnt.DecodeDKSHostFacts(inventoryData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks: inventory:", err)
		return 1
	}
	plan, err := dhnt.PlanDKSPlacement(pipeline, inventory.Facts, now, maxFactAge)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks:", err)
		return 1
	}
	output, err := dhnt.MarshalDKSPlacementPlan(plan)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks:", err)
		return 1
	}
	if _, err := os.Stdout.Write(output); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt plan-dks:", err)
		return 1
	}
	return 0
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
			if _, err := os.Stdout.Write(data); err != nil {
				fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
				return 1
			}
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
		if _, err := os.Stdout.Write(data); err != nil {
			fmt.Fprintln(os.Stderr, "bashy dhnt:", err)
			return 1
		}
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
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt emit-run:", err)
		return 1
	}
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
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintln(os.Stderr, "bashy dhnt aggregate:", err)
		return 1
	}
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
