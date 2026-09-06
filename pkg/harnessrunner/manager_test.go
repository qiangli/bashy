package harnessrunner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

func newTestManager(t *testing.T) (*Manager, []byte) {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef")
	manager, err := New(t.TempDir(), key)
	if err != nil {
		t.Fatal(err)
	}
	return manager, key
}

func authorizeRequest(t *testing.T, manager *Manager, req Request) Request {
	t.Helper()
	preflight := manager.Preflight(req)
	if preflight.Protocol != "ok" || preflight.Outcome != OutcomePrepared || preflight.Intent == nil {
		t.Fatalf("preflight = %#v", preflight)
	}
	authorization, err := manager.Authorize(*preflight.Intent, AuthorizationClaims{
		Binding: req.Binding, IntentDigest: preflight.Intent.Digest,
		PolicyRevision: 9, GrantDigest: "sha256:grant", ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	req.IntentDigest = preflight.Intent.Digest
	req.AuthorizationBinding = &authorization
	return req
}

func TestExecuteRequiresExactDigestBoundAuthorization(t *testing.T) {
	manager, _ := newTestManager(t)
	req := authorizeRequest(t, manager, compileRequest(t, "printf '%s' hello"))
	result := manager.Execute(context.Background(), req)
	if result.Protocol != "ok" || result.Outcome != OutcomeCompleted || result.Process == nil || result.Process.ExitCode == nil || *result.Process.ExitCode != 0 {
		t.Fatalf("execute = %#v", result)
	}
	if got := outputBytes(t, result.Output.Stdout); string(got) != "hello" {
		t.Fatalf("stdout = %q", got)
	}

	drifted := req
	drifted.RequestID = "req-drift"
	drifted.Command.Script = "printf '%s' changed"
	blocked := manager.Execute(context.Background(), drifted)
	if blocked.Protocol != "error" || blocked.Error == nil || blocked.Error.Code != "bindingMismatch" {
		t.Fatalf("drifted execution = %#v", blocked)
	}

	tampered := req
	tampered.Binding.StateRevision++
	blocked = manager.Execute(context.Background(), tampered)
	if blocked.Error == nil || blocked.Error.Code != "bindingMismatch" {
		t.Fatalf("tampered binding = %#v", blocked)
	}
}

func TestPreflightUsesKeyedSecretDigestAndNeverSerializesValue(t *testing.T) {
	manager, _ := newTestManager(t)
	req := compileRequest(t, "true")
	req.Command.Environment = []EnvironmentVariable{{Name: "API_TOKEN", ValueRef: "secret://api/token", Secret: true, Value: "do-not-persist"}}
	result := manager.Preflight(req)
	if result.Protocol != "ok" || result.Intent == nil || len(result.Intent.Environment) != 1 {
		t.Fatalf("preflight = %#v", result)
	}
	if got := result.Intent.Environment[0].ValueDigest; len(got) < len("hmac-sha256:") || got[:len("hmac-sha256:")] != "hmac-sha256:" {
		t.Fatalf("secret digest = %q", got)
	}
	if len(result.Effects) != 1 || result.Effects[0].Kind != "cred" || result.Effects[0].Target != "secret://api/token" {
		t.Fatalf("credential effects = %#v", result.Effects)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsString(string(encoded), "do-not-persist") {
		t.Fatalf("secret leaked in envelope: %s", encoded)
	}
	if _, err := CompileIntent(req); err == nil {
		t.Fatal("unkeyed compiler accepted a secret value")
	}
}

func TestExecuteSpoolsBinaryOutputAndReportsBound(t *testing.T) {
	manager, _ := newTestManager(t)
	req := compileRequest(t, `printf '\001\002abcdef'`)
	req.Limits.StdoutBytes = 4
	req = authorizeRequest(t, manager, req)
	result := manager.Execute(context.Background(), req)
	if result.Output == nil || !result.Output.Truncated || result.Output.OmittedBytes != 4 {
		t.Fatalf("output receipt = %#v", result.Output)
	}
	if got := outputBytes(t, result.Output.Stdout); string(got) != "\x01\x02ab" {
		t.Fatalf("binary stdout = %v", got)
	}
	encoded, err := json.Marshal(result)
	if err != nil || !json.Valid(encoded) {
		t.Fatalf("invalid JSON envelope: %v %q", err, encoded)
	}
}

func TestStartPollAndSequencedStdin(t *testing.T) {
	manager, _ := newTestManager(t)
	req := compileRequest(t, "read value; printf done")
	req.Limits.StdinBytes = 16
	req = authorizeRequest(t, manager, req)
	started := manager.Start(req)
	if started.Outcome != OutcomeRunning || started.Process == nil {
		t.Fatalf("start = %#v", started)
	}
	data := []byte("hello\n")
	inputRequest := Request{
		SchemaVersion: RequestSchemaVersion, RequestID: "stdin-1", Binding: req.Binding,
		JobID: started.Process.JobID, JobGeneration: started.Process.Generation,
		Input: &InputChunk{Sequence: 1, Encoding: "base64", Data: base64.StdEncoding.EncodeToString(data), Digest: digestBytes(data)},
	}
	accepted := manager.WriteStdin(inputRequest)
	if accepted.Protocol != "ok" || accepted.AcceptedInput == nil || *accepted.AcceptedInput != 1 {
		t.Fatalf("stdin = %#v", accepted)
	}
	duplicate := manager.WriteStdin(inputRequest)
	if duplicate.Protocol != "ok" || duplicate.AcceptedInput == nil {
		t.Fatalf("duplicate stdin was not idempotent: %#v", duplicate)
	}

	poll := Request{SchemaVersion: RequestSchemaVersion, RequestID: "poll-1", Binding: req.Binding, JobID: started.Process.JobID, JobGeneration: started.Process.Generation}
	result := pollUntilTerminal(t, manager, poll)
	if result.Outcome != OutcomeCompleted || string(outputBytes(t, result.Output.Stdout)) != "done" {
		t.Fatalf("terminal poll = %#v", result)
	}
	second := poll
	second.StdoutCursor = result.Output.StdoutNext
	if next := manager.Poll(second); len(next.Output.Stdout) != 0 || next.Output.StdoutNext != result.Output.StdoutNext {
		t.Fatalf("cursor was not monotonic: %#v", next.Output)
	}
}

func TestCancelAndRestartReconciliationAreTyped(t *testing.T) {
	manager, key := newTestManager(t)
	req := authorizeRequest(t, manager, compileRequest(t, "sleep 10"))
	started := manager.Start(req)
	control := Request{SchemaVersion: RequestSchemaVersion, RequestID: "cancel-1", Binding: req.Binding, JobID: started.Process.JobID, JobGeneration: started.Process.Generation}
	cancelled := manager.Cancel(control)
	if cancelled.Protocol != "ok" || cancelled.Outcome != OutcomeCancelled {
		t.Fatalf("cancel = %#v", cancelled)
	}
	terminal := pollUntilTerminal(t, manager, control)
	if terminal.Outcome != OutcomeCancelled {
		t.Fatalf("cancel settlement = %#v", terminal)
	}

	req2 := compileRequest(t, "sleep 10")
	req2.Binding.IdempotencyKey = "restart-job"
	req2 = authorizeRequest(t, manager, req2)
	started2 := manager.Start(req2)
	restarted, err := New(manager.root, key)
	if err != nil {
		t.Fatal(err)
	}
	poll := Request{SchemaVersion: RequestSchemaVersion, RequestID: "after-restart", Binding: req2.Binding, JobID: started2.Process.JobID, JobGeneration: started2.Process.Generation}
	unknown := restarted.Poll(poll)
	if unknown.Outcome != OutcomeAmbiguous {
		t.Fatalf("restart result = %#v", unknown)
	}
	_ = manager.Cancel(poll)
}

func TestSignalRejectsNonPortableValueAndBinding(t *testing.T) {
	manager, _ := newTestManager(t)
	req := authorizeRequest(t, manager, compileRequest(t, "sleep 10"))
	started := manager.Start(req)
	control := Request{SchemaVersion: RequestSchemaVersion, Binding: req.Binding, JobID: started.Process.JobID, JobGeneration: started.Process.Generation, Signal: Signal("SIGUSR1")}
	unsupported := manager.Signal(control)
	if unsupported.Error == nil || unsupported.Error.Code != "unsupported" || unsupported.Portability.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("unsupported signal = %#v", unsupported)
	}
	control.Signal = SignalTerminate
	control.Binding.RunID = "other"
	blocked := manager.Signal(control)
	if blocked.Error == nil || blocked.Error.Code != "bindingMismatch" {
		t.Fatalf("cross-run signal = %#v", blocked)
	}
	control.Binding = req.Binding
	_ = manager.Cancel(control)
}

func TestStrictWireDecoderRejectsUnknownAndTrailingInput(t *testing.T) {
	for _, input := range []string{
		`{"schemaVersion":"bashy.harness.request/v1alpha1","operation":"poll","surprise":true}`,
		`{"schemaVersion":"bashy.harness.request/v1alpha1","operation":"poll"}{}`,
		`{"schemaVersion":"old","operation":"poll"}`,
		`{"schemaVersion":"bashy.harness.request/v1alpha1","operation":"approve"}`,
	} {
		if _, err := DecodeRequest(bytes.NewBufferString(input)); err == nil {
			t.Fatalf("accepted invalid request %s", input)
		}
	}
	valid, err := DecodeRequest(bytes.NewBufferString(`{"schemaVersion":"bashy.harness.request/v1alpha1","operation":"poll"}`))
	if err != nil || valid.Operation != OperationPoll {
		t.Fatalf("valid request: %#v, %v", valid, err)
	}
}

func pollUntilTerminal(t *testing.T, manager *Manager, req Request) Result {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result := manager.Poll(req)
		if result.Outcome != OutcomeRunning {
			return result
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not settle")
	return Result{}
}

func outputBytes(t *testing.T, chunks []OutputChunk) []byte {
	t.Helper()
	var result []byte
	for _, chunk := range chunks {
		decoded, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, decoded...)
	}
	return result
}

func containsString(value, target string) bool {
	for i := 0; i+len(target) <= len(value); i++ {
		if value[i:i+len(target)] == target {
			return true
		}
	}
	return false
}
