// Package harnessrunner implements the effect-safe command boundary used by a
// YAML harness. It deliberately contains no workflow, approval, queue, agent,
// trigger, or sink policy.
package harnessrunner

import "time"

const (
	RequestSchemaVersion = "bashy.harness.request/v1alpha1"
	ResultSchemaVersion  = "bashy.harness.result/v1alpha1"
)

type Operation string

const (
	OperationPreflight Operation = "preflight"
	OperationExecute   Operation = "execute"
	OperationStart     Operation = "start"
	OperationPoll      Operation = "poll"
	OperationStdin     Operation = "stdin"
	OperationSignal    Operation = "signal"
	OperationCancel    Operation = "cancel"
)

type Outcome string

const (
	OutcomePrepared  Outcome = "prepared"
	OutcomeRunning   Outcome = "running"
	OutcomeCompleted Outcome = "completed"
	OutcomeBlocked   Outcome = "blocked"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeTimedOut  Outcome = "timedOut"
	OutcomeFailed    Outcome = "failed"
	OutcomeAmbiguous Outcome = "ambiguous"
)

type Binding struct {
	RunID               string `json:"runId"`
	NodeID              string `json:"nodeId"`
	Attempt             int    `json:"attempt"`
	StateRevision       uint64 `json:"stateRevision"`
	ConfigDigest        string `json:"configDigest"`
	LifecycleGeneration uint64 `json:"lifecycleGeneration"`
	IdempotencyKey      string `json:"idempotencyKey"`
}

type Command struct {
	Script      string                `json:"script,omitempty"`
	Argv        []string              `json:"argv,omitempty"`
	Cwd         string                `json:"cwd"`
	Environment []EnvironmentVariable `json:"environment,omitempty"`
}

// EnvironmentVariable keeps the value out of JSON while still binding its
// digest. Value is request-local input; ValueRef is the safe durable identity.
type EnvironmentVariable struct {
	Name     string `json:"name"`
	ValueRef string `json:"valueRef,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
	Value    string `json:"-"`
}

type Limits struct {
	WallTimeMs  int64 `json:"wallTimeMs,omitempty"`
	StdoutBytes int64 `json:"stdoutBytes,omitempty"`
	StderrBytes int64 `json:"stderrBytes,omitempty"`
	StdinBytes  int64 `json:"stdinBytes,omitempty"`
}

type Placement struct {
	ID         string `json:"id"`
	Generation uint64 `json:"generation"`
}

type Request struct {
	SchemaVersion        string                `json:"schemaVersion"`
	RequestID            string                `json:"requestId"`
	Operation            Operation             `json:"operation"`
	Binding              Binding               `json:"binding"`
	Command              Command               `json:"command,omitempty"`
	Limits               Limits                `json:"limits,omitempty"`
	Placement            Placement             `json:"placement"`
	IntentDigest         string                `json:"intentDigest,omitempty"`
	AuthorizationBinding *AuthorizationBinding `json:"authorizationBinding,omitempty"`
	JobID                string                `json:"jobId,omitempty"`
	JobGeneration        uint64                `json:"jobGeneration,omitempty"`
	StdoutCursor         int64                 `json:"stdoutCursor,omitempty"`
	StderrCursor         int64                 `json:"stderrCursor,omitempty"`
	MaxChunkBytes        int64                 `json:"maxChunkBytes,omitempty"`
	Input                *InputChunk           `json:"input,omitempty"`
	Signal               Signal                `json:"signal,omitempty"`
}

type InputChunk struct {
	Sequence uint64 `json:"sequence"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
	Digest   string `json:"digest"`
}

type Signal string

const (
	SignalInterrupt Signal = "interrupt"
	SignalTerminate Signal = "terminate"
	SignalKill      Signal = "kill"
)

type Effect struct {
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	Target    string `json:"target,omitempty"`
	Source    string `json:"source"`
	Certainty string `json:"certainty"`
}

type CommandFact struct {
	Name           string   `json:"name"`
	Argv           []string `json:"argv,omitempty"`
	Resolver       string   `json:"resolver"`
	Group          string   `json:"group,omitempty"`
	Tier           string   `json:"tier,omitempty"`
	Stage          string   `json:"stage,omitempty"`
	Shape          string   `json:"shape,omitempty"`
	Capabilities   []string `json:"capabilities,omitempty"`
	MaximumEffects []string `json:"maximumEffects,omitempty"`
	ContentDigest  string   `json:"contentDigest,omitempty"`
}

type UnsupportedFact struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type Intent struct {
	Digest       string            `json:"digest"`
	Complete     bool              `json:"complete"`
	AtlasDigest  string            `json:"atlasDigest"`
	ScriptDigest string            `json:"scriptDigest,omitempty"`
	Argv         []string          `json:"argv,omitempty"`
	Cwd          string            `json:"cwd"`
	Environment  []EnvironmentFact `json:"environment,omitempty"`
	Limits       Limits            `json:"limits"`
	Placement    Placement         `json:"placement"`
	Commands     []CommandFact     `json:"commands,omitempty"`
	Effects      []Effect          `json:"effects,omitempty"`
	Unsupported  []UnsupportedFact `json:"unsupported,omitempty"`
}

type EnvironmentFact struct {
	Name        string `json:"name"`
	ValueRef    string `json:"valueRef,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	ValueDigest string `json:"valueDigest"`
}

// AuthorizationClaims are supplied by the trusted graph interpreter after
// policy evaluation. The runner signs facts; it never decides policy.
type AuthorizationClaims struct {
	Binding        Binding   `json:"binding"`
	IntentDigest   string    `json:"intentDigest"`
	PolicyRevision uint64    `json:"policyRevision"`
	GrantDigest    string    `json:"grantDigest"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
}

type AuthorizationBinding struct {
	Claims AuthorizationClaims `json:"claims"`
	Token  string              `json:"token"`
}

type ProcessResult struct {
	JobID      string     `json:"jobId,omitempty"`
	Generation uint64     `json:"generation,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Signal     string     `json:"signal,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	DurationMs int64      `json:"durationMs,omitempty"`
}

type OutputChunk struct {
	From     int64  `json:"from"`
	To       int64  `json:"to"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

type OutputResult struct {
	Stdout       []OutputChunk `json:"stdout,omitempty"`
	Stderr       []OutputChunk `json:"stderr,omitempty"`
	StdoutNext   int64         `json:"stdoutNext"`
	StderrNext   int64         `json:"stderrNext"`
	Truncated    bool          `json:"truncated"`
	OmittedBytes int64         `json:"omittedBytes,omitempty"`
}

type Portability struct {
	Platform    string   `json:"platform"`
	Unsupported []string `json:"unsupported,omitempty"`
}

type ProtocolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Result struct {
	SchemaVersion string         `json:"schemaVersion"`
	RequestID     string         `json:"requestId"`
	Operation     Operation      `json:"operation"`
	Protocol      string         `json:"protocol"`
	Outcome       Outcome        `json:"outcome"`
	Binding       Binding        `json:"binding"`
	Intent        *Intent        `json:"intent,omitempty"`
	Effects       []Effect       `json:"effects,omitempty"`
	Process       *ProcessResult `json:"process,omitempty"`
	Output        *OutputResult  `json:"output,omitempty"`
	Portability   Portability    `json:"portability"`
	AcceptedInput *uint64        `json:"acceptedInputSequence,omitempty"`
	Error         *ProtocolError `json:"error,omitempty"`
}
