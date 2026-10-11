// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package tmpl is `bashy tmpl`: render a Go text/template file against JSON
// data. The function set is pure — string and JSON helpers only. There is no
// exec, env, file or network function, so rendering has the effect `read`
// and nothing more, which is what lets the `tmpl` fence declare it.
package tmpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"
)

const usage = `usage: bashy tmpl render FILE [--data JSON|-] [--data-file PATH]

Render the Go text/template FILE and print the result on stdout.

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
`

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
	file, data, dataFile, hasData, err := parseRender(rest)
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

func parseRender(args []string) (file, data, dataFile string, hasData bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--data" || a == "--data-file":
			if i+1 >= len(args) {
				return "", "", "", false, fmt.Errorf("%s needs a value", a)
			}
			i++
			if a == "--data" {
				data, hasData = args[i], true
			} else {
				dataFile = args[i]
			}
		case strings.HasPrefix(a, "--data="):
			data, hasData = strings.TrimPrefix(a, "--data="), true
		case strings.HasPrefix(a, "--data-file="):
			dataFile = strings.TrimPrefix(a, "--data-file=")
		case strings.HasPrefix(a, "-") && a != "-":
			return "", "", "", false, fmt.Errorf("unknown flag %q", a)
		case file == "":
			file = a
		default:
			return "", "", "", false, fmt.Errorf("unexpected argument %q", a)
		}
	}
	if file == "" {
		return "", "", "", false, errors.New("a template FILE is required")
	}
	return file, data, dataFile, hasData, nil
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
