package harnessrunner

import "testing"

func TestSessionAgentic(t *testing.T) {
	for _, tc := range []struct {
		env  []string
		want bool
	}{
		{[]string{"A=1", "BASHY_AGENTIC=1"}, true},
		{[]string{"BASHY_AGENTIC=true"}, true},
		{[]string{"BASHY_AGENTIC=0"}, false},
		{[]string{"PATH=/bin"}, false},
		{nil, false},
	} {
		if got := sessionAgentic(tc.env); got != tc.want {
			t.Errorf("sessionAgentic(%q) = %v, want %v", tc.env, got, tc.want)
		}
	}
}
