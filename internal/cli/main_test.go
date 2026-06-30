package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestNewRunnerInheritsOLDPWD(t *testing.T) {
	// bash (and every POSIX shell) carries an inherited $OLDPWD through so
	// `cd -` works in a fresh shell. Stripping it was a non-conformance
	// (oils builtin-cd differential vs all 5 oracle shells).
	t.Setenv("OLDPWD", "/tmp")
	r, err := newRunner()
	if err != nil {
		t.Fatal(err)
	}
	got := r.Env.Get("OLDPWD")
	if !got.IsSet() || got.String() != "/tmp" {
		t.Fatalf("OLDPWD not inherited into runner: set=%v val=%q", got.IsSet(), got.String())
	}
}

func TestCommandSubstOpenBefore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		pos  syntax.Pos
		want bool
	}{
		{
			name: "inside command substitution",
			src:  "echo $( if x; then echo foo )\n",
			pos:  syntax.NewPos(0, 1, 9),
			want: true,
		},
		{
			name: "after command substitution",
			src:  "echo $(echo ok) if x; then echo foo\n",
			pos:  syntax.NewPos(0, 1, 17),
			want: false,
		},
		{
			name: "plain compound command",
			src:  "if x; then echo foo )\n",
			pos:  syntax.NewPos(0, 1, 1),
			want: false,
		},
		{
			name: "arithmetic expansion",
			src:  "echo $((1 + ))\n",
			pos:  syntax.NewPos(0, 1, 11),
			want: false,
		},
		{
			name: "multiline command substitution",
			src:  "echo $(\nif x; then echo foo\n)\n",
			pos:  syntax.NewPos(0, 2, 1),
			want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := commandSubstOpenBefore([]byte(test.src), test.pos); got != test.want {
				t.Fatalf("commandSubstOpenBefore() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPrintBashParseErrorCompoundEOF(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		text string
		want string
	}{
		{
			name: "if non-empty eof line",
			src:  "if\n\ttrue; true\nthen\n\techo foo bar",
			text: "`if` statement must end with `fi`",
			want: "bash: -c: line 5: syntax error: unexpected end of file from `if' command on line 1\n",
		},
		{
			name: "until whitespace eof line",
			src:  "until false\ndo\n\techo false\n\t",
			text: "`until` statement must end with `done`",
			want: "bash: -c: line 4: syntax error: unexpected end of file from `until' command on line 1\n",
		},
		{
			name: "case whitespace eof line",
			src:  "case foo in\nbar)\tif false\n\tthen\n\t\ttrue\n\tfi\n\t;;\n\t",
			text: "`case` statement must end with `esac`",
			want: "bash: -c: line 7: syntax error: unexpected end of file from `case' command on line 1\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			printBashParseError(&buf, []byte(test.src), "bash: -c", syntax.ParseError{
				Pos:  syntax.NewPos(0, 1, 1),
				Text: test.text,
			})
			if got := buf.String(); got != test.want {
				t.Fatalf("printBashParseError mismatch\nwant:\n%q\ngot:\n%q", test.want, got)
			}
		})
	}
}

func TestPrintBashParseErrorArrayCompatibility(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		src        string
		pos        syntax.Pos
		text       string
		incomplete bool
		want       string
	}{
		{
			name: "array assignment after command word",
			src:  `printf "%s\n" -a a=(a 'b  c')`,
			pos:  syntax.NewPos(0, 1, 20),
			text: "a command can only contain words and redirects; encountered `(`",
			want: "bash: -c: line 1: syntax error near unexpected token `('\n" +
				"bash: -c: line 1: `printf \"%s\\n\" -a a=(a 'b  c')'\n",
		},
		{
			name: "empty declare name",
			src:  `declare -r []=asdf`,
			pos:  syntax.NewPos(0, 1, 13),
			text: "invalid var name",
			want: "bash: -c: line 1: declare: `[]=asdf': not a valid identifier\n",
		},
		{
			name: "zsh process substitution in declare",
			src:  `declare -a ''=(a 'b c')`,
			pos:  syntax.NewPos(0, 1, 14),
			text: "`=(` process substitutions are a zsh feature",
			want: "bash: -c: line 1: syntax error near unexpected token `('\n" +
				"bash: -c: line 1: `declare -a ''=(a 'b c')'\n",
		},
		{
			name: "array metacharacter pair",
			src:  `metas=( <> < > ! )`,
			pos:  syntax.NewPos(0, 1, 9),
			text: "array element values must be words",
			want: "bash: -c: line 1: syntax error near unexpected token `<>'\n" +
				"bash: -c: line 1: `metas=( <> < > ! )'\n",
		},
		{
			name: "arithmetic bare tilde",
			src:  `echo $(( ~ ))`,
			pos:  syntax.NewPos(0, 1, 10),
			text: "`~` must be followed by an expression",
			want: "bash: -c: line 1: ~ : arithmetic syntax error: operand expected (error token is \"~ \")\n",
		},
		{
			// `a[` — an array subscript running off the end of the input.
			// bash reports the matching-`]' EOF wording and omits the
			// source-line echo (array-assign__006).
			name:       "unterminated array subscript",
			src:        `a[`,
			pos:        syntax.NewPos(1, 1, 2),
			text:       "`[` must be followed by an expression",
			incomplete: true,
			want:       "bash: -c: line 1: unexpected EOF while looking for matching `]'\n",
		},
		{
			// A stray `)` where bash expects a command: bash names it as a
			// generic unexpected token, with the source-line echo (array__005).
			name: "stray close paren",
			src:  `)`,
			pos:  syntax.NewPos(0, 1, 1),
			text: "`)` can only be used to close a subshell",
			want: "bash: -c: line 1: syntax error near unexpected token `)'\n" +
				"bash: -c: line 1: `)'\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			printBashParseError(&buf, []byte(test.src), "bash: -c", syntax.ParseError{
				Pos:        test.pos,
				Text:       test.text,
				Incomplete: test.incomplete,
			})
			if got := buf.String(); got != test.want {
				t.Fatalf("printBashParseError mismatch\nwant:\n%q\ngot:\n%q", test.want, got)
			}
		})
	}
}

func TestDefaultCommandArgv0(t *testing.T) {
	t.Parallel()
	tests := []struct {
		arg0 string
		want string
	}{
		{"/tmp/bin/bash", "bash"},
		{"specialname", "specialname"},
		{"-specialname", "-specialname"},
	}
	for _, test := range tests {
		if got := defaultCommandArgv0(test.arg0); got != test.want {
			t.Fatalf("defaultCommandArgv0(%q) = %q, want %q", test.arg0, got, test.want)
		}
	}
}

func TestSplitCombinedShortFlagsLoginCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "single login",
			args: []string{"bashy", "-l", "-c", "echo ok"},
			want: []string{"bashy", "-login", "-c", "echo ok"},
		},
		{
			name: "combined login command",
			args: []string{"bashy", "-lc", "echo ok"},
			want: []string{"bashy", "-login", "-c", "echo ok"},
		},
		{
			name: "combined login errexit command",
			args: []string{"bashy", "-lec", "echo ok"},
			want: []string{"bashy", "-login", "-o", "errexit", "-c", "echo ok"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := splitCombinedShortFlags(test.args); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("splitCombinedShortFlags(%q) = %q, want %q", test.args, got, test.want)
			}
		})
	}
}

func TestStaticAliasExpand(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"shopt -s expand_aliases",
		"alias switch=case",
		"switch foo in foo) echo ok;; esac",
		"alias echo='echo ordinary'",
		"echo stays-runtime",
		"echo $( switch foo in foo) echo ok;; esac )",
		"alias comsub0='echo $(echo $DATE'",
		"comsub0)",
		"alias math1='echo $( date )'",
		"math1)",
		"alias number='echo 123'",
		"(( $(number) ))",
		"alias DONE='}'",
		"echo ok; DONE)",
		"alias let='let --'",
		"let '1 == 1'",
		"alias al=' '",
		"shopt -s expand_aliases 2>/dev/null",
		"al for x in y",
		"do echo $x",
		"done",
		"${THIS_SH} -c '",
		"shopt -s expand_aliases 2>/dev/null",
		"alias al=\" \"",
		"alias foo=bar",
		"alias for=echo",
		"al for foo in v",
		"do echo foo=$foo bar=$bar",
		"done' bash",
		"${THIS_SH} -o posix -c '",
		"shopt -s expand_aliases 2>/dev/null",
		"alias al=\" \"",
		"alias foo=bar",
		"al for foo in v",
		"do echo foo=$foo bar=$bar",
		"done' bash",
		`alias raw="echo 'Error:"`,
		`raw bar'`,
		"alias comment=#",
		"comment text after",
		`alias pipe='printf "%s\n" \'`,
		"pipe|cat",
		"alias semi=';'",
		"echo a semi echo b",
		"alias in='<'",
		"cat in file",
		"alias 'headplus=cat <<EOF",
		"hello'",
		"headplus",
		"world",
		"EOF",
		"alias head='cat <<END' body='head",
		"here-document",
		"END'",
		"body",
		"unalias -a",
		`raw again'`,
		"",
	}, "\n")
	want := strings.Join([]string{
		"shopt -s expand_aliases",
		"alias switch=case",
		"case foo in foo) echo ok;; esac",
		"alias echo='echo ordinary'",
		"echo stays-runtime",
		"echo $( case foo in foo) echo ok;; esac )",
		"alias comsub0='echo $(echo $DATE'",
		"echo $(echo $DATE)",
		"alias math1='echo $( date )'",
		"math1)",
		"alias number='echo 123'",
		"(( 123 ))",
		"alias DONE='}'",
		"echo ok; })",
		"alias let='let --'",
		"let '1 == 1'",
		"alias al=' '",
		"shopt -s expand_aliases 2>/dev/null",
		"  for x in y",
		"do echo $x",
		"done",
		"${THIS_SH} -c '",
		"shopt -s expand_aliases 2>/dev/null",
		"alias al=\" \"",
		"alias foo=bar",
		"alias for=echo",
		"al for foo in v",
		"do echo foo=$foo bar=$bar",
		"done' bash",
		"${THIS_SH} -o posix -c '",
		"shopt -s expand_aliases 2>/dev/null",
		"alias al=\" \"",
		"alias foo=bar",
		"  for foo in v",
		"do echo foo=$foo bar=$bar",
		"done' bash",
		`alias raw="echo 'Error:"`,
		`echo 'Error: bar'`,
		"alias comment=#",
		"# text after",
		`alias pipe='printf "%s\n" \'`,
		`printf "%s\n" \|cat`,
		"alias semi=';'",
		"echo a ; echo b",
		"alias in='<'",
		"cat < file",
		"alias 'headplus=cat <<EOF",
		"hello'",
		"cat <<EOF",
		"hello",
		"world",
		"EOF",
		"alias head='cat <<END' body='head",
		"here-document",
		"END'",
		"cat <<END",
		"here-document",
		"END",
		"unalias -a",
		`raw again'`,
		"",
	}, "\n")
	if got := string(staticAliasExpand([]byte(src), false)); got != want {
		t.Fatalf("staticAliasExpand mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestQuoteParamReplBackquotes(t *testing.T) {
	t.Parallel()
	src := []byte("printf '%s\\n' ${qpath//`printf '%s' \"\\\\\\\\\"`/}\n")
	want := "printf '%s\\n' /tmp/foo/bar\n"
	if got := string(quoteParamReplBackquotes(src)); got != want {
		t.Fatalf("quoteParamReplBackquotes mismatch\ngot:  %s\nwant: %s", got, want)
	}
}

func TestRunRetriesPosixAfterParsedPrefix(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"set -o posix",
		`echo 1 ${IFS+'}'z}`,
		`echo 2 "${IFS+'}'z}"`,
		`echo 3 "foo ${IFS+'bar} baz"`,
		`printf '%s\n' "4 foo ${IFS+"b   c"} baz"`,
		"",
	}, "\n")
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(r, strings.NewReader(src), "posixexp2.sub"); err != nil {
		t.Fatal(err)
	}
	want := "1 }z\n2 ''z}\n3 foo 'bar baz\n4 foo b   c baz\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s", want, got, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunKeepsBashConditionalAfterRuntimePosix(t *testing.T) {
	t.Parallel()
	src := strings.Join([]string{
		"set -o posix",
		"empty=",
		"if [[ $empty -gt 0 ]]; then echo bad; fi",
		"if [[ 1 -gt 0 ]]; then echo ok; fi",
		"",
	}, "\n")
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(r, strings.NewReader(src), "new-exp2.sub"); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "ok\n"; got != want {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s", want, got, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunQuotedEmptyHeredocEOFKeepsBody(t *testing.T) {
	t.Parallel()
	src := "cat <<''\nhi\nthere\n''\n"
	var stdout, stderr bytes.Buffer
	parser := syntax.NewParser(
		syntax.Variant(syntax.LangBash),
		syntax.HeredocEOFWarning(func(startLine, eofLine int, stop string) {
			stderr.WriteString("heredoc.tests: line 4: warning: here-document at line 1 delimited by end-of-file (wanted `')\n")
		}),
	)
	file, err := parser.Parse(strings.NewReader(src), "heredoc.tests")
	if err != nil {
		t.Fatal(err)
	}
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron("PATH=/usr/bin:/bin")),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	runErr := r.Run(context.Background(), file)
	if got, want := stdout.String(), "hi\nthere\n''\n"; got != want {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s\nrunErr:%v", want, got, stderr.String(), runErr)
	}
	wantErr := "heredoc.tests: line 4: warning: here-document at line 1 delimited by end-of-file (wanted `')\n"
	if got := stderr.String(); got != wantErr {
		t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q\nrunErr:%v", wantErr, got, runErr)
	}
	if runErr != nil {
		t.Fatalf("run error: %v\nstdout:\n%s\nstderr:\n%s", runErr, stdout.String(), stderr.String())
	}
}

func TestRunBadSubstDollarParamRecovery(t *testing.T) {
	src := "set -e\ntrap 'echo $?' EXIT\necho ${$NO_SUCH_VAR}\n"
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = oldStderr
		readStderr.Close()
	}()
	err = run(r, strings.NewReader(src), "./errors2.sub")
	writeStderr.Close()
	globalStderr, readErr := io.ReadAll(readStderr)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err == nil {
		t.Fatal("expected recovered parse error")
	}
	if want := "1\n"; stdout.String() != want {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected runner stderr: %s", stderr.String())
	}
	wantErr := "./errors2.sub: line 3: ${$NO_SUCH_VAR}: bad substitution\n"
	if string(globalStderr) != wantErr {
		t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q", wantErr, string(globalStderr))
	}
}

func TestRunArithmeticParseErrorRecoveryContinues(t *testing.T) {
	src := "echo $((a b))\n(( x=9 y=41 ))\necho after\n"
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = oldStderr
		readStderr.Close()
	}()
	err = run(r, strings.NewReader(src), "./arith.tests")
	writeStderr.Close()
	globalStderr, readErr := io.ReadAll(readStderr)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err == nil {
		t.Fatal("expected recovered parse error")
	}
	if got, want := stdout.String(), "after\n"; got != want {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s", want, got, stderr.String())
	}
	wantErr := "./arith.tests: line 1: a b: arithmetic syntax error in expression (error token is \"b\")\n" +
		"./arith.tests: line 2: ((: x=9 y=41 : arithmetic syntax error in expression (error token is \"y=41 \")\n"
	if string(globalStderr) != wantErr {
		t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q", wantErr, string(globalStderr))
	}
}

func TestRunNestedBadSubstRecoveryContinues(t *testing.T) {
	src := "c=\"\"\necho ${c//${$(($#-1))}/x/}\nset -- a b\nprintf '<%s>\\n' \"$@\"\n"
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = oldStderr
		readStderr.Close()
	}()
	err = run(r, strings.NewReader(src), "./new-exp.tests")
	writeStderr.Close()
	globalStderr, readErr := io.ReadAll(readStderr)
	if readErr != nil {
		t.Fatal(readErr)
	}
	// A nested bad substitution (`${$(($#-1))}`) is a RUNTIME expansion error in
	// bash, not a parse error: bash prints "bad substitution" to stderr, the
	// offending command is abandoned, the script CONTINUES, and the final exit
	// status is the last command's (0 here). Verified identical to bash 5.3
	// (stdout "<a>\n<b>\n", exit 0, the bad-subst diagnostic on stderr). Earlier
	// the engine modeled this as a recovered *parse* error (non-nil run() return
	// + diagnostic on os.Stderr); the engine was made more bash-faithful (runtime
	// expansion error on the runner's own stderr, run() returns nil / exit 0).
	if err != nil {
		t.Fatalf("want nil (exit 0, bash-correct), got err: %v", err)
	}
	wantOut := "<a>\n<b>\n"
	if stdout.String() != wantOut {
		t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q\nstderr:\n%s", wantOut, stdout.String(), stderr.String())
	}
	wantErr := "./new-exp.tests: line 2: ${$(($#-1))}: bad substitution\n"
	if stderr.String() != wantErr {
		t.Fatalf("runner stderr mismatch\nwant:\n%q\ngot:\n%q", wantErr, stderr.String())
	}
	if len(globalStderr) != 0 {
		t.Fatalf("unexpected os.Stderr output: %q", string(globalStderr))
	}
}

// TestRunUnexpectedTokenAbort pins bash 5.3's fatal "syntax error near
// unexpected token `X'" abort for a malformed case (no subject word) and a
// loop reserved word used outside a loop: bash runs nothing on the offending
// line and exits 2, where our generic recovery used to run the line's prefix
// and continue. The guard subtest confirms a complete statement on an earlier
// line still runs first (bash reads-parses-executes one command at a time).
func TestRunUnexpectedTokenAbort(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantOut    string
		wantErr    string
		wantStatus interp.ExitStatus
	}{
		{
			name:       "case-missing-subject",
			src:        "case\nin esac\n",
			wantStatus: 2,
			wantErr: "./s: line 1: syntax error near unexpected token `newline'\n" +
				"./s: line 1: `case'\n",
		},
		{
			name:       "keyword-out-of-loop-via-cmdsub",
			src:        "$(echo f)$(echo or) i in a b c; do echo $i; done\necho status=$?\n",
			wantStatus: 2,
			wantErr: "./s: line 1: syntax error near unexpected token `do'\n" +
				"./s: line 1: `$(echo f)$(echo or) i in a b c; do echo $i; done'\n",
		},
		{
			name:    "prior-line-statement-runs-first",
			src:     "echo hi\ncase\nin esac\n",
			wantOut: "hi\n",
			// Earlier-line `echo hi` runs; the up-front abort is declined and
			// the case error falls to the recovery path, which runs line 3's
			// `in` as a command (status 127), matching the pre-existing path.
			// (The "in: command not found" lands on the runner's own stderr,
			// not the os.Stderr the parse-error diagnostics use.)
			wantStatus: 127,
			wantErr: "./s: line 2: syntax error near unexpected token `case'\n" +
				"./s: line 2: `case'\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			r, err := interp.New(
				interp.StdIO(nil, &stdout, &stderr),
				interp.Env(expand.ListEnviron()),
				interp.WithBashCompatErrors(true),
			)
			if err != nil {
				t.Fatal(err)
			}
			oldStderr := os.Stderr
			readStderr, writeStderr, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stderr = writeStderr
			defer func() {
				os.Stderr = oldStderr
				readStderr.Close()
			}()
			runErr := run(r, strings.NewReader(tc.src), "./s")
			writeStderr.Close()
			globalStderr, readErr := io.ReadAll(readStderr)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var es interp.ExitStatus
			if !errors.As(runErr, &es) || es != tc.wantStatus {
				t.Fatalf("want ExitStatus(%d), got %v", tc.wantStatus, runErr)
			}
			if stdout.String() != tc.wantOut {
				t.Fatalf("stdout mismatch\nwant:\n%q\ngot:\n%q", tc.wantOut, stdout.String())
			}
			if string(globalStderr) != tc.wantErr {
				t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q", tc.wantErr, string(globalStderr))
			}
		})
	}
}

// TestRunStrayCloseParenAborts pins bash 5.3's handling of array__005:
//
//	a=(
//	1
//	&
//	'2 3'
//	)
//	argv.py "${a[@]}"
//
// bash recovers from the mid-array `&` (a control operator) — reporting it and
// running the following `'2 3'` as a command (status 127) — but the stray `)`
// is a FATAL parse error: bash names it as a generic unexpected token and
// aborts the whole input (status 2), so the trailing `argv.py` line never runs.
// Our generic statement recovery used to rewrite the `)` to "`)` can only be
// used to close a subshell" and run the trailing line; this guards both the
// bash wording and the abort. Verified byte-for-byte against bash 5.3.
func TestRunStrayCloseParenAborts(t *testing.T) {
	src := "a=(\n1\n&\n'2 3'\n)\nargv.py \"${a[@]}\"\n"
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = oldStderr
		readStderr.Close()
	}()
	runErr := run(r, strings.NewReader(src), "./s")
	writeStderr.Close()
	globalStderr, readErr := io.ReadAll(readStderr)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var es interp.ExitStatus
	if !errors.As(runErr, &es) || es != 2 {
		t.Fatalf("want ExitStatus(2) (fatal abort at `)`), got %v", runErr)
	}
	// argv.py never runs (aborted) — so stdout is empty and the runner's
	// stderr holds only the `'2 3'` command-not-found from the recovered line.
	if stdout.String() != "" {
		t.Fatalf("stdout should be empty (trailing line aborted), got %q", stdout.String())
	}
	wantRunnerErr := "./s: line 4: 2 3: command not found\n"
	if stderr.String() != wantRunnerErr {
		t.Fatalf("runner stderr mismatch\nwant:\n%q\ngot:\n%q", wantRunnerErr, stderr.String())
	}
	wantParseErr := "./s: line 3: syntax error near unexpected token `&'\n" +
		"./s: line 3: `&'\n" +
		"./s: line 5: syntax error near unexpected token `)'\n" +
		"./s: line 5: `)'\n"
	if string(globalStderr) != wantParseErr {
		t.Fatalf("parse-error stderr mismatch\nwant:\n%q\ngot:\n%q", wantParseErr, string(globalStderr))
	}
}

// TestRunBraceGroupEOFAbort pins bash 5.3's fatal "unclosed `{` command group
// at end of file" abort. A here-document delimited by EOF swallows the closing
// `}`, leaving the brace group opened on line 9 unterminated; bash warns about
// the here-document, then reports the unterminated group and exits 2 WITHOUT
// running the swallowed body. Our statement recovery used to re-parse and run
// that body instead, so this guards the up-front fatal classification.
func TestRunBraceGroupEOFAbort(t *testing.T) {
	src := "# c1\n# c2\n# c3\n# c4\n# c5\n# c6\n# c7\n# c8\nfun() {\n  cat << \"$@\"\nhi\n1 2\n}\nfun 1 2\n"
	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron()),
		interp.WithBashCompatErrors(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	oldStderr := os.Stderr
	readStderr, writeStderr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeStderr
	defer func() {
		os.Stderr = oldStderr
		readStderr.Close()
	}()
	runErr := run(r, strings.NewReader(src), "./s")
	writeStderr.Close()
	globalStderr, readErr := io.ReadAll(readStderr)
	if readErr != nil {
		t.Fatal(readErr)
	}
	var es interp.ExitStatus
	if !errors.As(runErr, &es) || es != 2 {
		t.Fatalf("want ExitStatus(2), got %v", runErr)
	}
	if stdout.String() != "" {
		t.Fatalf("want no stdout (body must not run), got %q", stdout.String())
	}
	wantErr := "./s: line 14: warning: here-document at line 10 delimited by end-of-file (wanted `$@')\n" +
		"./s: line 15: syntax error: unexpected end of file from `{' command on line 9\n"
	if string(globalStderr) != wantErr {
		t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q", wantErr, string(globalStderr))
	}
}

// TestRunBacktickComsubUnclosedQuote pins bash 5.3's quirk for a backtick
// command substitution with an unterminated quote in its body: bash reports a
// runtime "command substitution" error, the substitution expands to nothing,
// the enclosing `echo` still runs (one empty line), and the shell exits 0 — not
// the status-2 parse abort our eager parser would otherwise emit.
func TestRunBacktickComsubUnclosedQuote(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{"plain", "echo `echo \"`"},
		{"backslashes", "echo `echo \\\\\\\\\"`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origCommand := *command
			*command = tc.cmd
			t.Cleanup(func() { *command = origCommand })

			var stdout bytes.Buffer
			r, err := interp.New(
				interp.StdIO(nil, &stdout, io.Discard),
				interp.Env(expand.ListEnviron()),
				interp.CommandString(true),
				interp.WithBashCompatErrors(true),
			)
			if err != nil {
				t.Fatal(err)
			}
			oldStderr := os.Stderr
			readStderr, writeStderr, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stderr = writeStderr
			defer func() {
				os.Stderr = oldStderr
				readStderr.Close()
			}()
			runErr := run(r, strings.NewReader(*command), "bash")
			writeStderr.Close()
			globalStderr, readErr := io.ReadAll(readStderr)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if runErr != nil {
				t.Fatalf("want nil (exit 0, bash-correct), got %v", runErr)
			}
			if stdout.String() != "\n" {
				t.Fatalf("want one empty line on stdout, got %q", stdout.String())
			}
			wantErr := "bash: command substitution: line 1: unexpected EOF while looking for matching `\"'\n"
			if string(globalStderr) != wantErr {
				t.Fatalf("stderr mismatch\nwant:\n%q\ngot:\n%q", wantErr, string(globalStderr))
			}
		})
	}
}

// runForcedInteractiveInput drives runForcedInteractiveExec with raw
// input bytes (including readline control keys) on a pipe replacing
// os.Stdin, returning the runner's stdout.
func runForcedInteractiveInput(t *testing.T, input string) string {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		pw.WriteString(input)
		pw.Close()
	}()
	oldStdin := os.Stdin
	os.Stdin = pr
	t.Cleanup(func() { os.Stdin = oldStdin })

	var out bytes.Buffer
	r, err := interp.New(interp.StdIO(nil, &out, io.Discard), interp.Interactive(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := runForcedInteractiveExec(r); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestForcedInteractiveOperateAndGetNext(t *testing.T) {
	// Mirrors the last sections of bash's history4.sub: type a few
	// commands (one multi-line, one HISTIGNOREd), then C-p back three
	// entries and C-o twice. The recalled multi-line entry must rerun
	// whole, and each C-o must execute and fetch the next entry even
	// while HISTSIZE stifling shifts the list.
	t.Setenv("HISTSIZE", "6")
	t.Setenv("HISTFILE", "")
	t.Setenv("HISTIGNORE", "&:history*:fc*")
	out := runForcedInteractiveInput(t,
		"echo 0\necho 1\necho 2\necho \"(left\nmid\nright)\"\necho A\necho B\nhistory -w\n\x10\x10\x10\x0f\x0f\n")
	want := "0\n1\n2\n(left\nmid\nright)\nA\nB\n(left\nmid\nright)\nA\nB\n"
	if out != want {
		t.Errorf("operate-and-get-next:\n got: %q\nwant: %q", out, want)
	}
}

func TestForcedInteractiveReverseSearch(t *testing.T) {
	// Mirrors the first sections of history4.sub: load a HISTFILE whose
	// multi-line entry comes back as separate line entries, C-r search
	// for it, then C-o through the continuation lines and the next
	// entries. Clearing HISTFILE first must prevent the exit-time save
	// from clobbering the file.
	dir := t.TempDir()
	hf := filepath.Join(dir, "histfile")
	content := "echo 0\necho 1\necho 2\necho \"(left\nmid\nright)\"\necho A\necho B\nhistory -w\n"
	if err := os.WriteFile(hf, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTSIZE", "8")
	t.Setenv("HISTFILE", hf)
	t.Setenv("HISTIGNORE", "&:history*:fc*")
	out := runForcedInteractiveInput(t, "HISTFILE=\n\x12left\x0f\x0f\x0f\x0f\n")
	want := "(left\nmid\nright)\nA\nB\n"
	if out != want {
		t.Errorf("reverse search:\n got: %q\nwant: %q", out, want)
	}
	data, err := os.ReadFile(hf)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("history file clobbered at exit:\n got: %q\nwant: %q", string(data), content)
	}
}

func TestBashExecutionStringNotExported(t *testing.T) {
	if val, ok := os.LookupEnv("BASH_EXECUTION_STRING"); ok {
		t.Cleanup(func() {
			os.Setenv("BASH_EXECUTION_STRING", val)
		})
		os.Unsetenv("BASH_EXECUTION_STRING")
	}

	origCommand := *command
	*command = `echo "v=[$BASH_EXECUTION_STRING]"`
	t.Cleanup(func() {
		*command = origCommand
	})

	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = pw
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	r, err := newRunner()
	if err != nil {
		pw.Close()
		t.Fatal(err)
	}

	err = run(r, strings.NewReader(*command), "bash")
	pw.Close()
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, pr); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	want := `v=[echo "v=[$BASH_EXECUTION_STRING]"]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	pr2, pw2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = pw2

	r2, err := newRunner()
	if err != nil {
		pw2.Close()
		t.Fatal(err)
	}

	err = run(r2, strings.NewReader("env"), "bash")
	pw2.Close()
	if err != nil {
		t.Fatal(err)
	}

	var buf2 bytes.Buffer
	if _, err := io.Copy(&buf2, pr2); err != nil {
		t.Fatal(err)
	}

	envOutput := buf2.String()
	found := false
	for _, line := range strings.Split(envOutput, "\n") {
		if strings.HasPrefix(line, "BASH_EXECUTION_STRING=") {
			found = true
			break
		}
	}
	if found {
		t.Errorf("BASH_EXECUTION_STRING was exported to env:\n%s", envOutput)
	}
}
