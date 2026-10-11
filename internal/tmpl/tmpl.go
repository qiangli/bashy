// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package tmpl is `bashy tmpl`: render a Go text/template file against JSON
// data. The function set is pure — string and JSON helpers only. There is no
// exec, env, file or network function, so rendering has the effect `read`
// and nothing more, which is what lets the `tmpl` fence declare it.
package tmpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"

	"mvdan.cc/sh/v3/polyglot"
)

const usage = `usage: bashy tmpl render [--engine ENGINE] FILE [--data JSON|-] [--data-file PATH]

Render the template FILE and print the result on stdout. The default engine is
the built-in Go text/template.

  --engine ENGINE   render with a third-party engine instead (see Engine rod);
                    omitted or "go" is the built-in engine
  --data JSON       the template data: a JSON object ("-" reads it from stdin)
  --data-file PATH  the template data, read from a JSON file

Without data the template sees an empty object. A key the template names but
the data lacks is an error (missingkey=error), never an empty string.

Purity rule: the function set is the text/template builtins plus a small set
of pure helpers - upper lower trim replace split join toJson indent default
contains hasPrefix hasSuffix. There is no exec, env, file or network
function; a template naming one fails at parse time, so rendering only reads
its inputs (effect: read). Replace takes old, new, then the string:
{{ replace "a" "b" .s }}; default takes the fallback first: {{ default "x" .v }}
(it covers an empty or null value; a key absent from the data still errors).

Engine rod: a compliant third-party engine (jinja, mustache, handlebars, ...)
plugs in without bashy bundling it. In a Bash# unit:

  ~~~tmpl as T !ENGINE
  ...template...
  ~~~
  out := T.render('{"k":"v"}')

ENGINE is a Bash# function of the unit or a command registered with
"bashy commands add" - never a program found on PATH. The CLI form resolves
ENGINE the same way, except that a Bash# function lives in a unit and so is
reachable only through the fence; from the command line ENGINE is a registered
command (or a bashy verb). Compliance contract:

  ENGINE methods FILE        prints one JSON method object per line
                             ({"name":"render","effects":["exec"]}); the
                             list must include a render export
  ENGINE render FILE DATA_JSON
                             writes the rendered text to stdout; a non-zero
                             exit is an error

Any other engine is refused with "engine is not template-compliant"; there is
never a fallback to the Go engine. Purity is guaranteed only for the built-in
engine (effect: read). A third-party engine states its own effects in its
methods answer - exec and net are allowed - and those declared effects are
what a @guard reads, so they are visible and guardable.
`

// Engine runs one call of a third-party template engine: argv is
// "<verb> <file> [args...]" and the engine's stdout is the result; a non-zero
// exit is the error. stderr receives the engine's diagnostics.
type Engine func(ctx context.Context, argv []string, stderr io.Writer) (string, error)

// ResolveEngine is the embedder's resolver for `--engine NAME`: a registered
// command, never PATH. nil, or a miss, refuses the engine.
var ResolveEngine func(name string) (Engine, bool)

// Main runs `bashy tmpl`; the return value is the process exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if args[0] != "render" {
		fmt.Fprintf(stderr, "bashy tmpl: unknown subcommand %q (subcommands: render)\n", args[0])
		return 2
	}
	rest := args[1:]
	if len(rest) == 1 && (rest[0] == "-h" || rest[0] == "--help") {
		fmt.Fprint(stdout, usage)
		return 0
	}
	engine, file, data, dataFile, hasData, err := parseRender(rest)
	if err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: %v\n", err)
		return 2
	}
	var raw []byte
	switch {
	case hasData && dataFile != "":
		fmt.Fprintln(stderr, "bashy tmpl render: --data and --data-file are mutually exclusive")
		return 2
	case hasData && data == "-":
		raw, err = io.ReadAll(stdin)
	case hasData:
		raw = []byte(data)
	case dataFile != "":
		raw, err = os.ReadFile(dataFile)
	}
	if err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: reading data: %v\n", err)
		return 1
	}
	if engine != "" && engine != "go" {
		return renderWithEngine(engine, file, raw, stdout, stderr)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: %v\n", err)
		return 1
	}
	out, err := Render(file, string(src), raw)
	if err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: %v\n", err)
		return 1
	}
	_, _ = io.WriteString(stdout, out)
	return 0
}

func renderWithEngine(name, file string, rawData []byte, stdout, stderr io.Writer) int {
	var resolve Engine
	if ResolveEngine != nil {
		resolve, _ = ResolveEngine(name)
	}
	if resolve == nil {
		fmt.Fprintf(stderr, "bashy tmpl render: engine %q is not a registered command (PATH is never consulted)\n", name)
		return 1
	}
	if _, err := os.Stat(file); err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: %v\n", err)
		return 1
	}
	out, err := RenderWithEngine(context.Background(), resolve, name, file, rawData, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "bashy tmpl render: %v\n", err)
		return 1
	}
	_, _ = io.WriteString(stdout, out)
	return 0
}

// RenderWithEngine renders FILE through a third-party engine under the
// compliance contract. An engine that cannot answer "methods FILE" with a
// list holding a render export is refused - there is no fallback to the
// built-in engine. The data must be a JSON object (blank is {}).
func RenderWithEngine(ctx context.Context, engine Engine, name, file string, rawData []byte, stderr io.Writer) (string, error) {
	dataJSON := strings.TrimSpace(string(rawData))
	if dataJSON == "" {
		dataJSON = "{}"
	}
	var decoded any
	if err := json.Unmarshal([]byte(dataJSON), &decoded); err != nil {
		return "", fmt.Errorf("data is not valid JSON: %v", err)
	}
	if _, ok := decoded.(map[string]any); !ok {
		return "", errors.New("data must be a JSON object")
	}
	if stderr == nil {
		stderr = io.Discard
	}
	answer, err := engine(ctx, []string{polyglot.MethodsVerb, file}, stderr)
	if err != nil {
		return "", fmt.Errorf("engine is not template-compliant: %s methods: %v", name, err)
	}
	exports, err := polyglot.ParseMethods(name, answer)
	if err != nil {
		return "", fmt.Errorf("engine is not template-compliant: %v", err)
	}
	hasRender := false
	for _, export := range exports {
		hasRender = hasRender || export.Name == "render"
	}
	if !hasRender {
		return "", fmt.Errorf("engine is not template-compliant: %s lists no render export", name)
	}
	out, err := engine(ctx, []string{"render", file, dataJSON}, stderr)
	if err != nil {
		return "", fmt.Errorf("engine %s: render: %v", name, err)
	}
	return out, nil
}

func parseRender(args []string) (engine, file, data, dataFile string, hasData bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--data" || a == "--data-file":
			if i+1 >= len(args) {
				return "", "", "", "", false, fmt.Errorf("%s needs a value", a)
			}
			i++
			if a == "--data" {
				data, hasData = args[i], true
			} else {
				dataFile = args[i]
			}
		case a == "--engine":
			if i+1 >= len(args) {
				return "", "", "", "", false, errors.New("--engine needs a value")
			}
			i++
			engine = args[i]
		case strings.HasPrefix(a, "--engine="):
			engine = strings.TrimPrefix(a, "--engine=")
		case strings.HasPrefix(a, "--data="):
			data, hasData = strings.TrimPrefix(a, "--data="), true
		case strings.HasPrefix(a, "--data-file="):
			dataFile = strings.TrimPrefix(a, "--data-file=")
		case strings.HasPrefix(a, "-") && a != "-":
			return "", "", "", "", false, fmt.Errorf("unknown flag %q", a)
		case file == "":
			file = a
		default:
			return "", "", "", "", false, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if file == "" {
		return "", "", "", "", false, errors.New("a template FILE is required")
	}
	return engine, file, data, dataFile, hasData, nil
}

// Render executes the template source against the JSON data (empty or blank
// data is an empty object). name only labels errors.
func Render(name, src string, rawData []byte) (string, error) {
	var data any = map[string]any{}
	if len(bytes.TrimSpace(rawData)) > 0 {
		var decoded any
		if err := json.Unmarshal(rawData, &decoded); err != nil {
			return "", fmt.Errorf("data is not valid JSON: %v", err)
		}
		if _, ok := decoded.(map[string]any); !ok {
			return "", errors.New("data must be a JSON object")
		}
		data = decoded
	}
	t, err := template.New(name).Option("missingkey=error").Funcs(FuncMap()).Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// FuncMap is the pure helper set added to the text/template builtins.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trim":      strings.TrimSpace,
		"replace":   func(old, repl, s string) string { return strings.ReplaceAll(s, old, repl) },
		"split":     func(sep, s string) []string { return strings.Split(s, sep) },
		"join":      join,
		"toJson":    toJSON,
		"indent":    indent,
		"default":   defaultValue,
		"contains":  func(sub, s string) bool { return strings.Contains(s, sub) },
		"hasPrefix": func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"hasSuffix": func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },
	}
}

func join(sep string, list any) (string, error) {
	v := reflect.ValueOf(list)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return "", fmt.Errorf("join: want a list, got %T", list)
	}
	parts := make([]string, v.Len())
	for i := range parts {
		parts[i] = fmt.Sprint(v.Index(i).Interface())
	}
	return strings.Join(parts, sep), nil
}

func toJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func defaultValue(fallback, v any) any {
	if v == nil {
		return fallback
	}
	if rv := reflect.ValueOf(v); rv.IsZero() {
		return fallback
	}
	return v
}
