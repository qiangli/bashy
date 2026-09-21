package agentos

import (
	"reflect"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/yoke/pkg/fleet"
)

// The persisted argument schema, end to end through THIS repo: a record's
// `args:` (yoke fleet.Command.Args) is copied field for field onto
// interp.ResolvedCommand.Schema by registeredResolver, and the shell's ONE
// binder (sh interp/command_schema.go) then validates and binds an
// invocation before the exec rung — so a body is never entered on bad
// input, and a record without `args:` keeps the untyped pass-through argv.
// Nothing here binds; these tests prove the copy and the seam.
//
// The cases are the exact upstream PowerShell cases the two halves already
// pinned, re-run here against a PERSISTED record instead of an in-memory
// schema (sh) or a bare Validate (yoke):
//
//   - sh interp/command_resolver_test.go @ ef726acd3320b69e9aa6b90032c5a1d92464f2a0
//     TestRegisteredCommandSchemaPortedFromPowerShellValidateSet,
//     TestRegisteredCommandSchemaPortedFromPowerShellParameterBinding,
//     TestRegisteredCommandSchemaBindValidateInput (the `paint` fixture),
//     TestRegisteredCommandSchemaNilSchemaPreservesArgv.
//     Upstream: PowerShell/PowerShell @ 1e53f6bbab4b8791eae782474d21889f9e5d6038,
//     LICENSE.txt at the repo root is the MIT License.
//       test/powershell/Language/Classes/Scripting.Classes.Attributes.Tests.ps1
//         'Dynamically generated set works in PowerShell script with default
//          (immediate) cache expire'  /  'Get the appropriate error message'
//       test/powershell/Language/Scripting/ParameterBinding.Tests.ps1
//         "ValidateSet can use custom ErrorMessage"
//         'Multiple positional parameters case 1'
//       test/powershell/engine/ParameterBinding/ParameterBinding.Tests.ps1
//         "Should throw a exception when passing a string that can't be
//          parsed by Int"
//
//   - yoke pkg/fleet/command_schema_test.go @ 8163aa102a32282204d7e1ae949cd38b46509e5c
//     (merged as c9c29a5b734e4454ae3a6fc880b427202ad63b98)
//     TestCommandSchemaPortedPowerShellCases, paintSchema.
//     Upstream: PowerShell/PowerShell tag v7.5.3 @ b72c7ab1238c2d95b5c9004bca8399b8b3ca88ac,
//     LICENSE.txt at the repo root is the MIT License.
//       test/powershell/Language/Scripting/ParameterBinding.Tests.ps1
//         'Mandatory parameters used in non-interactive host'
//         'Parameter default value is converted correctly to the proper type
//          when nothing is set on parameter'
//         "ValidateSet can use custom ErrorMessage"
//       test/powershell/engine/ParameterBinding/ParameterBinding.Tests.ps1
//         "Verify that a SwitchParameter's IsPresent member is false if the
//          parameter is not specified"
//
// Script records re-enter bashy, which an in-process test cannot do, so the
// records here are exec records whose body is `sh` printing the argv it was
// entered with; the SAME schema on a script record is exercised against the
// built binary in commands_registry_e2e_test.go (TestE2ERegisteredScriptSchema).

// paintArgs is the shared fixture of both halves (sh
// TestRegisteredCommandSchemaBindValidateInput, yoke paintSchema).
func paintArgs() *fleet.CommandSchema {
	return &fleet.CommandSchema{
		Positionals: []fleet.CommandParameter{
			{Name: "color", Type: "string", Required: true, Enum: []string{"red", "blue"}},
			{Name: "level", Type: "int", Default: "3"},
		},
		Flags: []fleet.CommandFlag{
			{Name: "mode", Shorthand: "m", Type: "string", Default: "fast", Enum: []string{"fast", "slow"}},
			{Name: "count", Shorthand: "c", Type: "int", Required: true},
			{Name: "verbose", Shorthand: "v", Type: "bool"},
		},
	}
}

// writeBodyRecord persists an exec record whose body prints
// `entered|<arg>|<arg>…` — the argv the body was entered with, or nothing at
// all when the binder refused the call before the rung.
func writeBodyRecord(t *testing.T, name string, args *fleet.CommandSchema) {
	t.Helper()
	writeRecord(t, fleet.Command{
		Name: name,
		Exec: []string{shPath(t), "-c", `printf entered; for a; do printf '|%s' "$a"; done; echo`, name, fleet.ArgsToken},
		Args: args,
	})
}

// The resolver hands the shell the record's schema as a plain copy: every
// field of every positional and flag, in order; and a record without args
// resolves with a nil Schema, never an empty one.
func TestRegisteredResolverProjectsSchemaFieldForField(t *testing.T) {
	ringDir(t)
	writeBodyRecord(t, "paint", paintArgs())
	writeBodyRecord(t, "legacy", nil)

	rc, ok := registeredResolver("paint")
	if !ok || rc.Schema == nil {
		t.Fatalf("paint: resolved=%v schema=%v", ok, rc.Schema)
	}
	want := &interp.CommandSchema{
		Positionals: []interp.CommandParameter{
			{Name: "color", Type: "string", Required: true, Enum: []string{"red", "blue"}},
			{Name: "level", Type: "int", Default: "3"},
		},
		Flags: []interp.CommandFlag{
			{Name: "mode", Shorthand: "m", Type: "string", Default: "fast", Enum: []string{"fast", "slow"}},
			{Name: "count", Shorthand: "c", Type: "int", Required: true},
			{Name: "verbose", Shorthand: "v", Type: "bool"},
		},
	}
	if !reflect.DeepEqual(rc.Schema, want) {
		t.Errorf("schema not copied field for field:\n got %+v\nwant %+v", rc.Schema, want)
	}
	// The copy is a copy: the record's enum is not aliased by the schema.
	rec, _ := registeredLookup("paint")
	rc.Schema.Positionals[0].Enum[0] = "mutated"
	if rec.Args.Positionals[0].Enum[0] != "red" {
		t.Error("the projected schema aliases the record's enum slice")
	}
	if rc, ok := registeredResolver("legacy"); !ok || rc.Schema != nil {
		t.Errorf("legacy: resolved=%v schema=%+v, want resolved with nil schema", ok, rc.Schema)
	}
	// The mirror is exact: the two declarations must not drift apart.
	for _, pair := range [][2]reflect.Type{
		{reflect.TypeOf(fleet.CommandSchema{}), reflect.TypeOf(interp.CommandSchema{})},
		{reflect.TypeOf(fleet.CommandParameter{}), reflect.TypeOf(interp.CommandParameter{})},
		{reflect.TypeOf(fleet.CommandFlag{}), reflect.TypeOf(interp.CommandFlag{})},
	} {
		if pair[0].NumField() != pair[1].NumField() {
			t.Fatalf("%s has %d fields, %s has %d", pair[0], pair[0].NumField(), pair[1], pair[1].NumField())
		}
		for i := 0; i < pair[0].NumField(); i++ {
			f, g := pair[0].Field(i), pair[1].Field(i)
			if f.Name != g.Name || f.Type.Kind() != g.Type.Kind() {
				t.Errorf("field %d: %s.%s (%s) vs %s.%s (%s)", i, pair[0], f.Name, f.Type, pair[1], g.Name, g.Type)
			}
		}
	}
}

// A persisted schema is honoured in-shell, in both modes: valid typed / enum
// / defaulted arguments reach the body already bound and converted, an
// invalid call is refused with status 2 before the body is entered, and a
// legacy record keeps its argv byte for byte.
func TestRegisteredSchemaBindsBeforeTheBody(t *testing.T) {
	ringDir(t)
	writeBodyRecord(t, "paint", paintArgs())
	writeRecord(t, fleet.Command{ // an alias resolves the same schema
		Name: "paintalias", Aliases: []string{"pa"}, Args: paintArgs(),
		Exec: []string{shPath(t), "-c", `printf entered; for a; do printf '|%s' "$a"; done; echo`, "paintalias", fleet.ArgsToken},
	})
	writeBodyRecord(t, "legacy", nil)

	cases := []struct {
		name    string
		src     string
		wantOut string
		wantErr string
	}{
		// sh TestRegisteredCommandSchemaBindValidateInput, on a persisted record.
		{"typed enum default", "paint --mode slow -c 007 blue\n", "entered|--mode=slow|--count=7|blue|3\n", ""},
		{"defaults and bool flag", "paint --count=2 --verbose red\n", "entered|--mode=fast|--count=2|--verbose=true|red|3\n", ""},
		{"missing required", "paint red; echo rc=$?\n", "rc=2\n", "missing required flag --count"},
		{"extra positional", "paint --count 1 red 2 extra; echo rc=$?\n", "rc=2\n", "too many positional arguments"},
		{"enum refused", "paint --count 1 green; echo rc=$?\n", "rc=2\n", `color must be one of red, blue, got "green"`},
		{"conversion error", "paint --count nope red; echo rc=$?\n", "rc=2\n", `--count expects int, got "nope"`},
		{"alias binds the same schema", "pa -c 1 red\n", "entered|--mode=fast|--count=1|red|3\n", ""},
		{"alias refuses the same input", "pa red; echo rc=$?\n", "rc=2\n", "missing required flag --count"},
		// sh TestRegisteredCommandSchemaNilSchemaPreservesArgv: flag-shaped
		// tokens the runner never inspects, byte for byte.
		{"legacy preserves argv", "legacy --count nope -x --mode=slow -- green extra\n", "entered|--count|nope|-x|--mode=slow|--|green|extra\n", ""},
	}
	for _, posix := range []bool{false, true} {
		for _, tc := range cases {
			out, errOut, _ := runRegisteredSource(t, tc.src, posix)
			if out != tc.wantOut {
				t.Errorf("posix=%v %s: stdout=%q want %q (stderr=%q)", posix, tc.name, out, tc.wantOut, errOut)
			}
			if tc.wantErr == "" && errOut != "" {
				t.Errorf("posix=%v %s: stderr=%q want empty", posix, tc.name, errOut)
			}
			if tc.wantErr != "" && !strings.Contains(errOut, tc.wantErr) {
				t.Errorf("posix=%v %s: stderr=%q want substring %q", posix, tc.name, errOut, tc.wantErr)
			}
		}
	}
	// A refused call is a status, not a Run error: the script goes on.
	_, _, err := runRegisteredSource(t, "paint red\n", false)
	if st, ok := interp.IsExitStatus(err); !ok || st != 2 {
		t.Errorf("refused call as the last command: err=%v, want exit status 2", err)
	}
	// The certification profile sees neither the record nor its schema.
	t.Setenv("VSC_PROFILE", "cert")
	if rc, ok := registeredResolver("paint"); ok || rc.Schema != nil {
		t.Errorf("cert profile resolved paint: %+v", rc)
	}
}

// The ported PowerShell cases (provenance in the file header), each against
// a persisted record and the shell's binder, through the resolver copy.
func TestRegisteredSchemaPortedPowerShellCases(t *testing.T) {
	ringDir(t)

	// Scripting.Classes.Attributes.Tests.ps1, 'ValidateSet support a
	// dynamically generated set': the set ("Test1","TestString1","Test2")
	// from GenValuesForParam.GetValidValues(); `-Param1 "TestString1"` is
	// returned exactly, `-Param1 "TestStringWrong"` throws
	// ParameterArgumentValidationError,Get-TestValidateSetPS4.
	writeBodyRecord(t, "get-testvalidatesetps4", &fleet.CommandSchema{
		Positionals: []fleet.CommandParameter{{Name: "Param1", Type: "string", Required: true, Enum: []string{"Test1", "TestString1", "Test2"}}},
	})
	// Scripting/ParameterBinding.Tests.ps1, "ValidateSet can use custom
	// ErrorMessage": enum ('A','B','C'), `get-fook -p 2` throws with
	// "... Item '2' is not in '...'".
	writeBodyRecord(t, "get-fook", &fleet.CommandSchema{
		Positionals: []fleet.CommandParameter{{Name: "p", Type: "string", Required: true, Enum: []string{"A", "B", "C"}}},
	})
	// engine/ParameterBinding/ParameterBinding.Tests.ps1, "Should throw a
	// exception when passing a string that can't be parsed by Int":
	// `test-singleintparameter -Parameter1 'exampleInvalidParam'` throws
	// ParameterArgumentTransformationError naming both the parameter and
	// the value.
	writeBodyRecord(t, "test-singleintparameter", &fleet.CommandSchema{
		Flags: []fleet.CommandFlag{{Name: "Parameter1", Type: "int"}},
	})
	// Scripting/ParameterBinding.Tests.ps1, 'Multiple positional parameters
	// case 1': `get-foo -b d c` and `get-foo c -b d` both yield 'c,d' — a
	// named flag and a bare positional bind the same regardless of order.
	writeBodyRecord(t, "get-foo", &fleet.CommandSchema{
		Flags:       []fleet.CommandFlag{{Name: "b", Type: "string"}},
		Positionals: []fleet.CommandParameter{{Name: "a", Type: "string", Required: true}},
	})
	// Scripting/ParameterBinding.Tests.ps1, 'Parameter default value is
	// converted correctly to the proper type when nothing is set on
	// parameter': the default is a value OF the declared type — "007" as an
	// int reaches the body as 7.
	writeBodyRecord(t, "get-fooa", &fleet.CommandSchema{
		Positionals: []fleet.CommandParameter{{Name: "n", Type: "int", Default: "007"}},
	})
	// engine/ParameterBinding/ParameterBinding.Tests.ps1, "Verify that a
	// SwitchParameter's IsPresent member is false if the parameter is not
	// specified": absent, the switch binds nothing; present, it is true.
	writeBodyRecord(t, "sw", &fleet.CommandSchema{
		Flags: []fleet.CommandFlag{{Name: "Parameter1", Type: "bool"}},
	})
	// Scripting/ParameterBinding.Tests.ps1, 'Mandatory parameters used in
	// non-interactive host': a mandatory parameter the call omits is an
	// ERROR, never a prompt.
	writeBodyRecord(t, "get-mandatory", &fleet.CommandSchema{
		Positionals: []fleet.CommandParameter{{Name: "p", Required: true}},
	})

	cases := []struct {
		name    string
		src     string
		wantOut string
		wantErr []string
	}{
		{"ValidateSet accepts a value from the set", "get-testvalidatesetps4 TestString1\n", "entered|TestString1\n", nil},
		{"ValidateSet rejects before the body", "get-testvalidatesetps4 TestStringWrong; echo rc=$?\n", "rc=2\n", []string{`Param1 must be one of Test1, TestString1, Test2, got "TestStringWrong"`}},
		{"ValidateSet custom ErrorMessage case", "get-fook 2; echo rc=$?\n", "rc=2\n", []string{`p must be one of A, B, C, got "2"`}},
		{"int conversion names parameter and value", "test-singleintparameter --Parameter1 exampleInvalidParam; echo rc=$?\n", "rc=2\n", []string{"Parameter1", `"exampleInvalidParam"`}},
		{"positional case 1: flag first", "get-foo --b d c\n", "entered|--b=d|c\n", nil},
		{"positional case 1: positional first", "get-foo c --b d\n", "entered|--b=d|c\n", nil},
		{"default converted to the declared type", "get-fooa\n", "entered|7\n", nil},
		{"switch absent is not present", "sw\n", "entered\n", nil},
		{"switch present is true", "sw --Parameter1\n", "entered|--Parameter1=true\n", nil},
		{"mandatory omitted is an error", "get-mandatory; echo rc=$?\n", "rc=2\n", []string{"missing required positional p"}},
	}
	for _, tc := range cases {
		out, errOut, _ := runRegisteredSource(t, tc.src, false)
		if out != tc.wantOut {
			t.Errorf("%s: stdout=%q want %q (stderr=%q)", tc.name, out, tc.wantOut, errOut)
		}
		if len(tc.wantErr) == 0 && errOut != "" {
			t.Errorf("%s: stderr=%q want empty", tc.name, errOut)
		}
		for _, want := range tc.wantErr {
			if !strings.Contains(errOut, want) {
				t.Errorf("%s: stderr=%q want substring %q", tc.name, errOut, want)
			}
		}
	}

	// yoke's write-time half, through bashy's own catalog: "ValidateSet can
	// use custom ErrorMessage" — a default outside the set is refused when
	// the record is written, naming the set in declaration order, so the
	// binder never meets a record that fails every call.
	err := registeredCatalog().SaveCommand(fleet.Command{
		Name: "get-fook-bad", Script: "true", Effects: []string{"pure"},
		Args: &fleet.CommandSchema{Flags: []fleet.CommandFlag{{Name: "p", Default: "2", Enum: []string{"A", "B", "C"}}}},
	})
	if err == nil || !strings.Contains(err.Error(), `"2" is not one of A|B|C`) {
		t.Errorf("an invalid schema must be refused at write time: %v", err)
	}
	if _, ok := registeredLookup("get-fook-bad"); ok {
		t.Error("a refused record must not be in the ring")
	}
}
