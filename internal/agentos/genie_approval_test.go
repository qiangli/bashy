package agentos

// Sprint: #329; Story: #1837; Story-ID: bba673f54037

import (
	"os"
	"testing"

	"github.com/qiangli/yoke/pkg/agentlaunch"
)

func TestGenieApplyApproval(t *testing.T) {
	containerized := agentlaunch.Containerized
	agentlaunch.Containerized = func() bool { return false }
	t.Cleanup(func() { agentlaunch.Containerized = containerized })

	for _, tc := range []struct {
		name, allowance, approval, want string
		approvalSet                     bool
	}{
		{name: "unattended", allowance: "1", want: "auto"},
		{name: "explicit prompt", allowance: "1", approval: "prompt", approvalSet: true, want: "prompt"},
		{name: "no allowance", allowance: "0"},
		{name: "explicit empty", allowance: "1", approval: "", approvalSet: true, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(agentlaunch.UnsafeLaunchEnv, tc.allowance)
			t.Setenv("GENIE_APPROVAL", tc.approval)
			if !tc.approvalSet {
				if err := os.Unsetenv("GENIE_APPROVAL"); err != nil {
					t.Fatal(err)
				}
			}

			genieApplyApproval()
			got, set := os.LookupEnv("GENIE_APPROVAL")
			if set != (tc.approvalSet || tc.want != "") || got != tc.want {
				t.Fatalf("GENIE_APPROVAL = %q (set=%v), want %q (set=%v)", got, set, tc.want, tc.approvalSet || tc.want != "")
			}
		})
	}
}
