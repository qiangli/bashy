package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/qiangli/yoke/pkg/weave"
)

// The master's sync route names every call with the same task id and
// idempotency key the pull route would use; outpost forwards both headers.
const (
	mcpGatewayTaskHeader = "X-Gateway-Task-Id"
	mcpGatewayKeyHeader  = "X-Gateway-Idempotency-Key"
)

// mcpGatewayState answers a duplicate sync call whose first execution has
// not recorded a result yet; the result follows on the pull route.
type mcpGatewayState struct {
	State  string `json:"state"`
	TaskID string `json:"task_id"`
}

// mcpGatewayExecute is the --outpost Policy.Execute hook. It runs after the
// policy, so a gated call has already answered {state: "gated"} without a
// claim and the pull route keeps owning approvals. An executing call must
// carry both gateway headers (fail closed) and claims the host record the
// pull conductor shares, so a pull fallback with the same key never runs
// the command a second time.
func mcpGatewayExecute(ctx context.Context, call *mcpsdk.CallToolRequest, next func(context.Context) (mcpsdk.Result, error)) (mcpsdk.Result, error) {
	var header http.Header
	if call.Extra != nil {
		header = call.Extra.Header
	}
	taskID := strings.TrimSpace(header.Get(mcpGatewayTaskHeader))
	key := strings.TrimSpace(header.Get(mcpGatewayKeyHeader))
	if taskID == "" || key == "" {
		return mcpGatewayRefusal("refused: " + mcpGatewayTaskHeader + " and " + mcpGatewayKeyHeader + " are required"), nil
	}
	existing, fresh, err := weave.ClaimGatewayTaskOnHost(key, taskID)
	if err != nil {
		return mcpGatewayRefusal("refused: gateway claim: " + err.Error()), nil
	}
	if !fresh {
		return mcpGatewayReplay(key, existing), nil
	}
	res, err := next(ctx)
	text, exit, reply := mcpGatewayOutcome(res, err)
	if cerr := weave.CompleteGatewayTaskOnHost(key, taskID, text, exit, true, reply); cerr != nil {
		fmt.Fprintln(os.Stderr, "bashy mcp: record gateway result:", cerr)
	}
	return res, err
}

func mcpGatewayRefusal(text string) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: text}},
	}
}

// mcpGatewayReplay answers a duplicate key from the host record without
// executing: the first sync reply verbatim, a pull-route result in run_tool
// shape, or the task's current state.
func mcpGatewayReplay(key, taskID string) *mcpsdk.CallToolResult {
	task, err := weave.GatewayTaskOnHost(key)
	if err != nil {
		return mcpGatewayRefusal("refused: gateway record: " + err.Error())
	}
	if task.HasResult {
		if len(task.SyncReply) > 0 {
			var res mcpsdk.CallToolResult
			if json.Unmarshal(task.SyncReply, &res) == nil {
				return &res
			}
		}
		return &mcpsdk.CallToolResult{
			IsError:           task.Exit != 0,
			Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: task.Text}},
			StructuredContent: map[string]any{"stdout": task.Text, "stderr": "", "exit_code": task.Exit},
		}
	}
	return &mcpsdk.CallToolResult{
		IsError:           true,
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: "task " + task.TaskID + " is " + task.State + "; not executed again"}},
		StructuredContent: mcpGatewayState{State: task.State, TaskID: task.TaskID},
	}
}

// mcpGatewayOutcome maps a sync reply to the host record's text and exit.
// run_tool and direct tools report stdout/stderr/exit_code; any other tool
// falls back to its text content and error flag.
func mcpGatewayOutcome(res mcpsdk.Result, err error) (text string, exit int, reply []byte) {
	if err != nil {
		return err.Error(), 1, nil
	}
	call, ok := res.(*mcpsdk.CallToolResult)
	if !ok || call == nil {
		return "", 0, nil
	}
	reply, _ = json.Marshal(call)
	if raw, merr := json.Marshal(call.StructuredContent); merr == nil {
		var out struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode *int   `json:"exit_code"`
		}
		if json.Unmarshal(raw, &out) == nil && out.ExitCode != nil {
			return out.Stdout + out.Stderr, *out.ExitCode, reply
		}
	}
	var b strings.Builder
	for _, c := range call.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if call.IsError {
		exit = 1
	}
	return b.String(), exit, reply
}
