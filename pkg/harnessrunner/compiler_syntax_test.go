package harnessrunner

import (
	"strings"
	"testing"
)

// A command that does not parse compiles to an incomplete intent whose
// unsupported fact carries the parse error; it is never a request failure.
func TestCompileScriptSyntaxErrorIsUnsupported(t *testing.T) {
	var intent Intent
	intent.Complete = true
	if err := compileScript("foo(", &intent); err != nil {
		t.Fatalf("parse error returned as an error: %v", err)
	}
	if intent.Complete || len(intent.Unsupported) != 1 {
		t.Fatalf("intent: %+v", intent)
	}
	fact := intent.Unsupported[0]
	if fact.Kind != "syntax" || fact.Value != "foo(" || !strings.Contains(fact.Reason, "must be followed by") {
		t.Fatalf("fact: %+v", fact)
	}
}
