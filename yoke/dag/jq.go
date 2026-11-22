// Duplicated from github.com/qiangli/yoke/cmds/jq/jq.go under the operator's
// standalone-adapter rule (see yoke/COUPLED-EXCEPTIONS.md): no non-internal
// command package imports another command package, so yoke/dag registers its
// own pure-Go jq applet onto coreutils/tool for in-process DAG target bodies
// (such as genie's dag.md recipes) while AssertStandaloneDeps stays clean.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/itchyny/gojq"
	"github.com/qiangli/coreutils/tool"
)

var jqTool = &tool.Tool{
	Name:     "jq",
	Synopsis: "Filter and transform JSON, backed by pure-Go gojq.",
	Usage:    "jq [-c] [-e] [-n] [-r] [filter] [file ...]",
}

func init() { jqTool.Run = runJQ; tool.Register(jqTool) }

func runJQ(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(jqTool.Name)
	compact := fs.BoolP("compact-output", "c", false, "output without pretty-printing")
	exitStatus := fs.BoolP("exit-status", "e", false, "exit 1 when the last value is false or null")
	nullInput := fs.BoolP("null-input", "n", false, "use null as the single input value")
	rawOutput := fs.BoolP("raw-output", "r", false, "output raw strings, not JSON strings")
	operands, code := tool.Parse(rc, jqTool, fs, args)
	if code >= 0 {
		return code
	}

	filter := "."
	if len(operands) > 0 {
		filter = operands[0]
		operands = operands[1:]
	}

	query, err := gojq.Parse(filter)
	if err != nil {
		fmt.Fprintf(rc.Err, "jq: invalid query: %v\n", err)
		return 2
	}
	compiled, err := gojq.Compile(query, gojq.WithEnvironLoader(func() []string { return rc.Env }))
	if err != nil {
		fmt.Fprintf(rc.Err, "jq: compile error: %v\n", err)
		return 2
	}

	inputs, ok := readJQInputs(rc, operands, *nullInput)
	if !ok {
		return 1
	}

	var saw bool
	var last any
	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for _, input := range inputs {
		iter := compiled.RunWithContext(ctx, input)
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, ok := v.(error); ok {
				if halt, ok := err.(*gojq.HaltError); ok && halt.Value() == nil {
					break
				}
				fmt.Fprintf(rc.Err, "jq: %v\n", err)
				return 1
			}
			if err := writeJQValue(rc.Out, v, *rawOutput, *compact); err != nil {
				fmt.Fprintf(rc.Err, "jq: %v\n", err)
				return 1
			}
			saw = true
			last = v
		}
	}

	if *exitStatus {
		if !saw {
			return 4
		}
		if last == nil || last == false {
			return 1
		}
	}
	return 0
}

func readJQInputs(rc *tool.RunContext, names []string, nullInput bool) ([]any, bool) {
	if nullInput {
		return []any{nil}, true
	}
	if len(names) == 0 {
		vals, err := decodeJQJSONValues(rc.In, "<stdin>")
		if err != nil {
			fmt.Fprintf(rc.Err, "jq: %v\n", err)
			return nil, false
		}
		return vals, true
	}

	var vals []any
	for _, name := range names {
		var r io.Reader
		var closer io.Closer
		if name == "-" {
			r = rc.In
		} else {
			f, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "jq: %s: %v\n", name, err)
				return nil, false
			}
			r, closer = f, f
		}
		fileVals, err := decodeJQJSONValues(r, name)
		if closer != nil {
			_ = closer.Close()
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "jq: %v\n", err)
			return nil, false
		}
		vals = append(vals, fileVals...)
	}
	return vals, true
}

func decodeJQJSONValues(r io.Reader, name string) ([]any, error) {
	if r == nil {
		r = bytes.NewReader(nil)
	}
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var vals []any
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				return vals, nil
			}
			return nil, fmt.Errorf("invalid json in %s: %w", name, err)
		}
		vals = append(vals, v)
	}
}

func writeJQValue(w io.Writer, v any, raw, compact bool) error {
	if raw {
		if s, ok := v.(string); ok {
			_, err := fmt.Fprintln(w, s)
			return err
		}
	}
	b, err := gojq.Marshal(v)
	if err != nil {
		return err
	}
	if !compact {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, b, "", "  "); err == nil {
			b = pretty.Bytes()
		}
	}
	_, err = w.Write(append(b, '\n'))
	return err
}
