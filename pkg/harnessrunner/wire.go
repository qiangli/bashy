package harnessrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// DecodeRequest is the strict JSON boundary. Unknown fields and trailing JSON
// are rejected so a newer caller cannot accidentally run under older,
// narrower semantics.
func DecodeRequest(reader io.Reader) (Request, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, err
	}
	if request.SchemaVersion != RequestSchemaVersion {
		return Request{}, fmt.Errorf("unsupported request schema %q", request.SchemaVersion)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Request{}, fmt.Errorf("multiple JSON values are not allowed")
		}
		return Request{}, fmt.Errorf("trailing JSON: %w", err)
	}
	if !validOperation(request.Operation) {
		return Request{}, fmt.Errorf("unsupported operation %q", request.Operation)
	}
	return request, nil
}

func validOperation(operation Operation) bool {
	switch operation {
	case OperationPreflight, OperationExecute, OperationStart, OperationPoll, OperationStdin, OperationSignal, OperationCancel:
		return true
	default:
		return false
	}
}

// Handle dispatches only the seven mechanism operations. Policy evaluation
// and authorization issuance remain trusted caller responsibilities.
func (m *Manager) Handle(ctx context.Context, request Request) Result {
	switch request.Operation {
	case OperationPreflight:
		return m.Preflight(request)
	case OperationExecute:
		return m.Execute(ctx, request)
	case OperationStart:
		return m.Start(request)
	case OperationPoll:
		return m.Poll(request)
	case OperationStdin:
		return m.WriteStdin(request)
	case OperationSignal:
		return m.Signal(request)
	case OperationCancel:
		return m.Cancel(request)
	default:
		return failResult(m.baseResult(request), "unsupported", "unsupported operation", false)
	}
}
