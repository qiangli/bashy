package agentos

import (
	"os"
	"testing"
)

func runAccountDoor(t *testing.T, args ...string) (code int) {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"bashy", "account"}, args...)
	frontDoorObserving.Store(true)
	defer func() {
		frontDoorObserving.Store(false)
		os.Args = oldArgs
	}()
	func() {
		defer func() {
			v := recover()
			if v == nil {
				t.Error("account door returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatch()
	}()
	return code
}

func TestDispatchAccountInProcess(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	for _, args := range [][]string{{"schema", "--json"}, {"list", "--json"}} {
		if code := runAccountDoor(t, args...); code != 0 {
			t.Errorf("bashy account %v exited %d, want 0", args, code)
		}
	}
}

func TestAccountIsAlwaysShimmedAndSchemaed(t *testing.T) {
	if cobraVerb("account") == nil {
		t.Fatal("cobraVerb(account) is nil")
	}
	if !isFrontDoorInvocation("account") {
		t.Fatal("account is not a front-door verb")
	}
}
