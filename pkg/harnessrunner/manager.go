package harnessrunner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

const supervisorRecordVersion = "bashy.harness.job/v1alpha1"

type Manager struct {
	root string
	key  []byte
	mu   sync.Mutex
	jobs map[string]*liveJob
}

type liveJob struct {
	record  *jobRecord
	ctx     context.Context
	cancel  context.CancelFunc
	input   *io.PipeReader
	stdin   *io.PipeWriter
	inputMu sync.Mutex
	stdout  *boundedSpool
	stderr  *boundedSpool
	done    chan struct{}
}

type jobRecord struct {
	SchemaVersion string            `json:"schemaVersion"`
	JobID         string            `json:"jobId"`
	Generation    uint64            `json:"generation"`
	IntentDigest  string            `json:"intentDigest"`
	Intent        Intent            `json:"intent"`
	Binding       Binding           `json:"binding"`
	Limits        Limits            `json:"limits"`
	Outcome       Outcome           `json:"outcome"`
	StartedAt     time.Time         `json:"startedAt"`
	FinishedAt    *time.Time        `json:"finishedAt,omitempty"`
	ExitCode      *int              `json:"exitCode,omitempty"`
	Signal        string            `json:"signal,omitempty"`
	StdoutBytes   int64             `json:"stdoutBytes"`
	StderrBytes   int64             `json:"stderrBytes"`
	OmittedBytes  int64             `json:"omittedBytes"`
	StdinBytes    int64             `json:"stdinBytes"`
	InputDigests  map[uint64]string `json:"inputDigests,omitempty"`
}

func New(root string, authorizationKey []byte) (*Manager, error) {
	if len(authorizationKey) < 32 {
		return nil, fmt.Errorf("authorization key must contain at least 32 bytes")
	}
	if root == "" {
		return nil, fmt.Errorf("private runner root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return nil, err
	}
	return &Manager{root: abs, key: append([]byte(nil), authorizationKey...), jobs: make(map[string]*liveJob)}, nil
}

func (m *Manager) Preflight(req Request) Result {
	req.Operation = OperationPreflight
	result := m.baseResult(req)
	intent, err := compileIntent(req, m.key)
	if err != nil {
		return failResult(result, "invalidRequest", err.Error(), false)
	}
	result.Intent = &intent
	result.Effects = append([]Effect(nil), intent.Effects...)
	if !intent.Complete {
		result.Outcome = OutcomeBlocked
		result.Error = &ProtocolError{Code: "incompletePreflight", Message: "intent contains unclassified or dynamic effects", Retryable: false}
		result.Protocol = "error"
		return result
	}
	result.Outcome = OutcomePrepared
	return result
}

// Authorize signs caller-supplied policy facts. Calling this method is the
// trusted graph interpreter's assertion that policy evaluation allowed the
// exact complete intent; Manager itself contains no allow/deny rules.
func (m *Manager) Authorize(intent Intent, claims AuthorizationClaims) (AuthorizationBinding, error) {
	if !intent.Complete || intent.Digest == "" {
		return AuthorizationBinding{}, fmt.Errorf("cannot authorize an incomplete intent")
	}
	if claims.IntentDigest != intent.Digest {
		return AuthorizationBinding{}, fmt.Errorf("claim intent digest does not match preflight")
	}
	if claims.Binding.RunID == "" || claims.Binding.NodeID == "" || claims.Binding.IdempotencyKey == "" || claims.Binding.ConfigDigest == "" {
		return AuthorizationBinding{}, fmt.Errorf("authorization binding is incomplete")
	}
	token, err := signJSON(m.key, claims)
	if err != nil {
		return AuthorizationBinding{}, err
	}
	return AuthorizationBinding{Claims: claims, Token: token}, nil
}

func (m *Manager) Execute(ctx context.Context, req Request) Result {
	req.Operation = OperationExecute
	job, result, ok := m.begin(ctx, req)
	if !ok {
		return result
	}
	<-job.done
	return m.resultForRecord(req, job.record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes)
}

func (m *Manager) Start(req Request) Result {
	req.Operation = OperationStart
	job, result, ok := m.begin(context.Background(), req)
	if !ok {
		return result
	}
	return m.resultForRecord(req, job.record, 0, 0, req.MaxChunkBytes)
}

func (m *Manager) begin(parent context.Context, req Request) (*liveJob, Result, bool) {
	result := m.baseResult(req)
	intent, authErr := m.authorizedIntent(req)
	if authErr != nil {
		return nil, failResult(result, authErr.code, authErr.message, false), false
	}
	result.Intent = &intent
	result.Effects = append([]Effect(nil), intent.Effects...)
	jobID := m.jobID(req.AuthorizationBinding.Claims)

	m.mu.Lock()
	if existing := m.jobs[jobID]; existing != nil {
		m.mu.Unlock()
		if existing.record.IntentDigest != intent.Digest {
			return nil, failResult(result, "bindingMismatch", "idempotency key is already bound to another intent", false), false
		}
		return existing, m.resultForRecord(req, existing.record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes), false
	}
	if record, err := m.loadRecord(jobID); err == nil {
		m.mu.Unlock()
		if record.IntentDigest != intent.Digest {
			return nil, failResult(result, "bindingMismatch", "idempotency key is already bound to another intent", false), false
		}
		return nil, m.resultForRecord(req, record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes), false
	}
	job, err := m.reserve(req, intent, jobID, parent)
	if err != nil {
		m.mu.Unlock()
		return nil, failResult(result, "unavailable", err.Error(), true), false
	}
	m.jobs[jobID] = job
	m.mu.Unlock()
	go m.run(req, intent, job)
	return job, result, true
}

type authorizationError struct{ code, message string }

func (m *Manager) authorizedIntent(req Request) (Intent, *authorizationError) {
	if req.AuthorizationBinding == nil {
		return Intent{}, &authorizationError{"authorizationRequired", "authorization binding is required"}
	}
	auth := req.AuthorizationBinding
	if !verifyJSON(m.key, auth.Claims, auth.Token) {
		return Intent{}, &authorizationError{"bindingMismatch", "authorization signature is invalid"}
	}
	if !auth.Claims.ExpiresAt.IsZero() && !time.Now().UTC().Before(auth.Claims.ExpiresAt.UTC()) {
		return Intent{}, &authorizationError{"bindingMismatch", "authorization binding has expired"}
	}
	if auth.Claims.Binding != req.Binding {
		return Intent{}, &authorizationError{"bindingMismatch", "request binding differs from authorization"}
	}
	intent, err := compileIntent(req, m.key)
	if err != nil {
		return Intent{}, &authorizationError{"invalidRequest", err.Error()}
	}
	if !intent.Complete {
		return Intent{}, &authorizationError{"incompletePreflight", "intent contains unclassified or dynamic effects"}
	}
	if intent.Digest != req.IntentDigest || intent.Digest != auth.Claims.IntentDigest {
		return Intent{}, &authorizationError{"bindingMismatch", "request intent differs from authorization"}
	}
	return intent, nil
}

func (m *Manager) reserve(req Request, intent Intent, jobID string, parent context.Context) (*liveJob, error) {
	dir := m.jobDir(jobID)
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}
	stdout, err := newBoundedSpool(filepath.Join(dir, "stdout.bin"), req.Limits.StdoutBytes)
	if err != nil {
		return nil, err
	}
	stderr, err := newBoundedSpool(filepath.Join(dir, "stderr.bin"), req.Limits.StderrBytes)
	if err != nil {
		_ = stdout.Close()
		return nil, err
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if req.Limits.WallTimeMs > 0 {
		ctx, cancel = context.WithTimeout(parent, time.Duration(req.Limits.WallTimeMs)*time.Millisecond)
	} else {
		ctx, cancel = context.WithCancel(parent)
	}
	stdinReader, stdinWriter := io.Pipe()
	record := &jobRecord{
		SchemaVersion: supervisorRecordVersion, JobID: jobID, Generation: 1,
		IntentDigest: intent.Digest, Intent: intent, Binding: req.Binding, Outcome: OutcomeRunning,
		StartedAt: time.Now().UTC(), InputDigests: make(map[uint64]string),
	}
	record.Limits = req.Limits
	job := &liveJob{record: record, ctx: ctx, cancel: cancel, input: stdinReader, stdin: stdinWriter, stdout: stdout, stderr: stderr, done: make(chan struct{})}
	if err := m.persistRecord(record); err != nil {
		cancel()
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, err
	}
	return job, nil
}

func (m *Manager) run(req Request, intent Intent, job *liveJob) {
	defer close(job.done)
	defer job.input.Close()
	defer job.stdin.Close()
	defer job.cancel()

	env := make([]string, 0, len(req.Command.Environment))
	for _, value := range req.Command.Environment {
		env = append(env, value.Name+"="+value.Value)
	}
	sort.Strings(env)
	script := req.Command.Script
	if script == "" {
		script = quoteArgv(req.Command.Argv)
	}
	ctx := job.ctx
	termination := cli.RunSessionCommandResultWithConfig(ctx, cli.SessionIO{
		Command: script, Dir: intent.Cwd, Env: env, Stdin: job.input,
		Stdout: job.stdout, Stderr: job.stderr,
	}, cli.SessionConfig{WireExec: agentos.WireSessionExec(false), Preamble: agentos.Preamble})
	_ = job.stdout.Close()
	_ = job.stderr.Close()
	stdoutBytes, stdoutOmitted := job.stdout.stats()
	stderrBytes, stderrOmitted := job.stderr.stats()
	now := time.Now().UTC()

	m.mu.Lock()
	job.record.StdoutBytes = stdoutBytes
	job.record.StderrBytes = stderrBytes
	job.record.OmittedBytes = stdoutOmitted + stderrOmitted
	job.record.FinishedAt = &now
	job.record.ExitCode = &termination.ExitCode
	job.record.Signal = termination.Signal
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		job.record.Outcome = OutcomeTimedOut
	case errors.Is(ctx.Err(), context.Canceled):
		job.record.Outcome = OutcomeCancelled
	default:
		job.record.Outcome = OutcomeCompleted
	}
	_ = m.persistRecord(job.record)
	delete(m.jobs, job.record.JobID)
	m.mu.Unlock()
}

func (m *Manager) Poll(req Request) Result {
	req.Operation = OperationPoll
	result := m.baseResult(req)
	record, err := m.findRecord(req.JobID)
	if err != nil {
		return failResult(result, "jobNotFound", "job does not exist", false)
	}
	if req.JobGeneration != 0 && req.JobGeneration != record.Generation {
		return failResult(result, "staleGeneration", "job generation does not match", false)
	}
	if record.Binding != req.Binding {
		return failResult(result, "bindingMismatch", "job belongs to a different run binding", false)
	}
	if record.Outcome == OutcomeRunning {
		m.mu.Lock()
		live := m.jobs[record.JobID] != nil
		m.mu.Unlock()
		if !live {
			record.Outcome = OutcomeAmbiguous
		}
	}
	return m.resultForRecord(req, record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes)
}

func (m *Manager) WriteStdin(req Request) Result {
	req.Operation = OperationStdin
	result := m.baseResult(req)
	if req.Input == nil {
		return failResult(result, "invalidRequest", "input chunk is required", false)
	}
	if req.Input.Encoding != "base64" {
		return failResult(result, "unsupported", "only base64 stdin chunks are supported", false)
	}
	data, err := base64.StdEncoding.DecodeString(req.Input.Data)
	if err != nil || digestBytes(data) != req.Input.Digest {
		return failResult(result, "invalidRequest", "stdin data or digest is invalid", false)
	}
	m.mu.Lock()
	job := m.jobs[req.JobID]
	if job == nil {
		m.mu.Unlock()
		return failResult(result, "unavailable", "job is not attached to this supervisor", true)
	}
	job.inputMu.Lock()
	defer job.inputMu.Unlock()
	if req.JobGeneration != job.record.Generation {
		m.mu.Unlock()
		return failResult(result, "staleGeneration", "job generation does not match", false)
	}
	if job.record.Binding != req.Binding {
		m.mu.Unlock()
		return failResult(result, "bindingMismatch", "job belongs to a different run binding", false)
	}
	if job.record.Limits.StdinBytes <= 0 || job.record.StdinBytes+int64(len(data)) > job.record.Limits.StdinBytes {
		m.mu.Unlock()
		return failResult(result, "outputLimit", "stdin chunk exceeds the admitted limit", false)
	}
	if prior, ok := job.record.InputDigests[req.Input.Sequence]; ok {
		m.mu.Unlock()
		if prior != req.Input.Digest {
			return failResult(result, "bindingMismatch", "stdin sequence was already used with different content", false)
		}
		seq := req.Input.Sequence
		result.Outcome, result.AcceptedInput = OutcomeRunning, &seq
		return result
	}
	expected := uint64(len(job.record.InputDigests) + 1)
	if req.Input.Sequence != expected {
		m.mu.Unlock()
		return failResult(result, "invalidRequest", fmt.Sprintf("stdin sequence must be %d", expected), false)
	}
	m.mu.Unlock()
	if _, err := job.stdin.Write(data); err != nil {
		return failResult(result, "unavailable", "job stdin is closed", false)
	}
	m.mu.Lock()
	job.record.InputDigests[req.Input.Sequence] = req.Input.Digest
	job.record.StdinBytes += int64(len(data))
	_ = m.persistRecord(job.record)
	m.mu.Unlock()
	seq := req.Input.Sequence
	result.Outcome, result.AcceptedInput = OutcomeRunning, &seq
	return result
}

func (m *Manager) Signal(req Request) Result {
	req.Operation = OperationSignal
	if req.Signal != SignalInterrupt && req.Signal != SignalTerminate && req.Signal != SignalKill {
		return failResult(m.baseResult(req), "unsupported", "unsupported portable signal", false)
	}
	return m.stop(req, false)
}

func (m *Manager) Cancel(req Request) Result {
	req.Operation = OperationCancel
	return m.stop(req, true)
}

func (m *Manager) stop(req Request, cancellation bool) Result {
	result := m.baseResult(req)
	m.mu.Lock()
	job := m.jobs[req.JobID]
	if job == nil {
		m.mu.Unlock()
		record, err := m.loadRecord(req.JobID)
		if err != nil {
			return failResult(result, "jobNotFound", "job does not exist", false)
		}
		if record.Binding != req.Binding {
			return failResult(result, "bindingMismatch", "job belongs to a different run binding", false)
		}
		return m.resultForRecord(req, record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes)
	}
	if req.JobGeneration != job.record.Generation {
		m.mu.Unlock()
		return failResult(result, "staleGeneration", "job generation does not match", false)
	}
	if job.record.Binding != req.Binding {
		m.mu.Unlock()
		return failResult(result, "bindingMismatch", "job belongs to a different run binding", false)
	}
	job.cancel()
	_ = job.stdin.Close()
	record := job.record
	m.mu.Unlock()
	// A successful stop receipt is also the lifecycle barrier: callers may
	// immediately close the manager's control root without racing the worker's
	// final spool close and durable record rename.
	<-job.done
	result = m.resultForRecord(req, record, req.StdoutCursor, req.StderrCursor, req.MaxChunkBytes)
	if cancellation {
		result.Outcome = OutcomeCancelled
	}
	return result
}

func (m *Manager) baseResult(req Request) Result {
	return Result{
		SchemaVersion: ResultSchemaVersion, RequestID: req.RequestID,
		Operation: req.Operation, Protocol: "ok", Outcome: OutcomeFailed,
		Binding: req.Binding, Portability: Portability{Platform: runtime.GOOS + "/" + runtime.GOARCH},
	}
}

func failResult(result Result, code, message string, retryable bool) Result {
	result.Protocol = "error"
	result.Outcome = OutcomeBlocked
	result.Error = &ProtocolError{Code: code, Message: message, Retryable: retryable}
	return result
}

func (m *Manager) resultForRecord(req Request, record *jobRecord, stdoutCursor, stderrCursor, maximum int64) Result {
	m.mu.Lock()
	recordCopy := *record
	m.mu.Unlock()
	record = &recordCopy
	result := m.baseResult(req)
	result.Outcome = record.Outcome
	result.Intent = &record.Intent
	result.Effects = append([]Effect(nil), record.Intent.Effects...)
	result.Process = &ProcessResult{JobID: record.JobID, Generation: record.Generation, ExitCode: record.ExitCode, Signal: record.Signal, StartedAt: &record.StartedAt, FinishedAt: record.FinishedAt}
	if record.FinishedAt != nil {
		result.Process.DurationMs = record.FinishedAt.Sub(record.StartedAt).Milliseconds()
	}
	stdout, stdoutNext, stdoutErr := readChunk(filepath.Join(m.jobDir(record.JobID), "stdout.bin"), stdoutCursor, maximum)
	stderr, stderrNext, stderrErr := readChunk(filepath.Join(m.jobDir(record.JobID), "stderr.bin"), stderrCursor, maximum)
	if stdoutErr != nil || stderrErr != nil {
		return failResult(result, "invalidRequest", firstError(stdoutErr, stderrErr).Error(), false)
	}
	result.Output = &OutputResult{Stdout: stdout, Stderr: stderr, StdoutNext: stdoutNext, StderrNext: stderrNext, Truncated: record.OmittedBytes > 0, OmittedBytes: record.OmittedBytes}
	return result
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func (m *Manager) findRecord(jobID string) (*jobRecord, error) {
	if !validJobID(jobID) {
		return nil, fmt.Errorf("invalid job id")
	}
	m.mu.Lock()
	if live := m.jobs[jobID]; live != nil {
		record := *live.record
		m.mu.Unlock()
		return &record, nil
	}
	m.mu.Unlock()
	return m.loadRecord(jobID)
}

func (m *Manager) jobID(claims AuthorizationClaims) string {
	mac := hmac.New(sha256.New, m.key)
	_, _ = mac.Write([]byte(claims.Binding.RunID + "\x00" + claims.Binding.NodeID + "\x00" + fmt.Sprint(claims.Binding.Attempt) + "\x00" + claims.Binding.IdempotencyKey))
	return "job_" + hex.EncodeToString(mac.Sum(nil)[:16])
}

func validJobID(value string) bool {
	if !strings.HasPrefix(value, "job_") || len(value) != 36 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "job_"))
	return err == nil
}

func (m *Manager) jobDir(jobID string) string { return filepath.Join(m.root, jobID) }

func (m *Manager) persistRecord(record *jobRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := filepath.Join(m.jobDir(record.JobID), "job.json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (m *Manager) loadRecord(jobID string) (*jobRecord, error) {
	if !validJobID(jobID) {
		return nil, fmt.Errorf("invalid job id")
	}
	data, err := os.ReadFile(filepath.Join(m.jobDir(jobID), "job.json"))
	if err != nil {
		return nil, err
	}
	var record jobRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.SchemaVersion != supervisorRecordVersion || record.JobID != jobID {
		return nil, fmt.Errorf("invalid job record")
	}
	return &record, nil
}

func quoteArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, value := range argv {
		parts[i] = "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return strings.Join(parts, " ")
}
