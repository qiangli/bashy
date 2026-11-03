//go:build darwin

package agentos

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/yoke/pkg/fleet"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func TestRegisteredHandlerPreservesProcessSubstitution(t *testing.T) {
	for _, kind := range []string{"env", "cwd"} {
		t.Run(kind, func(t *testing.T) {
			ringDir(t)
			rec := fleet.Command{Name: "fdreader", Exec: []string{"/bin/cat", fleet.ArgsToken}}
			if kind == "env" {
				rec.Env = []string{"REGISTERED_FD_TEST=1"}
			} else {
				rec.Cwd = t.TempDir()
			}
			writeRecord(t, rec)
			var out, errout bytes.Buffer
			r, err := interp.New(interp.StdIO(nil, &out, &errout), interp.ExecHandlers(registeredHandler()))
			if err != nil {
				t.Fatal(err)
			}
			file, err := syntax.NewParser().Parse(strings.NewReader("fdreader <(echo inherited)"), "")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := r.Run(ctx, file); err != nil {
				t.Fatalf("run: %v stderr=%q", err, errout.String())
			}
			if got := out.String(); got != "inherited\n" {
				t.Fatalf("output=%q stderr=%q", got, errout.String())
			}
		})
	}
}
