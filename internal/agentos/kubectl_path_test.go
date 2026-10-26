package agentos

import (
	"os"
	"reflect"
	"testing"

	"mvdan.cc/sh/v3/pathconv"
)

func TestKubectlPathAndEnvConversion(t *testing.T) {
	// Verify environment variable conversion for KUBECONFIG and related variables.
	t.Setenv("KUBECONFIG", "/c/Users/liqia/AppData/Local/Temp/tmp.X")
	t.Setenv("OUTPOST_KUBECONFIG_PATH", "/c/Users/liqia/.kube/outpost.yaml")
	t.Setenv("DKS_PEER_KUBECONFIG", "/c/Users/liqia/.kube/peer.yaml")
	t.Setenv("DKS_FEDERATION_KUBECONFIG", "/c/Users/liqia/.kube/fed.yaml")

	adaptKubeEnvironment()

	if got := os.Getenv("KUBECONFIG"); got != `C:\Users\liqia\AppData\Local\Temp\tmp.X` {
		t.Errorf("KUBECONFIG = %q, want C:\\Users\\liqia\\AppData\\Local\\Temp\\tmp.X", got)
	}
	if got := os.Getenv("OUTPOST_KUBECONFIG_PATH"); got != `C:\Users\liqia\.kube\outpost.yaml` {
		t.Errorf("OUTPOST_KUBECONFIG_PATH = %q, want C:\\Users\\liqia\\.kube\\outpost.yaml", got)
	}
	if got := os.Getenv("DKS_PEER_KUBECONFIG"); got != `C:\Users\liqia\.kube\peer.yaml` {
		t.Errorf("DKS_PEER_KUBECONFIG = %q, want C:\\Users\\liqia\\.kube\\peer.yaml", got)
	}
	if got := os.Getenv("DKS_FEDERATION_KUBECONFIG"); got != `C:\Users\liqia\.kube\fed.yaml` {
		t.Errorf("DKS_FEDERATION_KUBECONFIG = %q, want C:\\Users\\liqia\\.kube\\fed.yaml", got)
	}

	// Verify argument conversion for kubectl / helm command line flags.
	drives := pathconv.DrivesOf("C")
	cases := []struct {
		in   []string
		want []string
	}{
		{
			in:   []string{"--kubeconfig", "/c/Users/liqia/AppData/Local/Temp/tmp.X", "get", "nodes"},
			want: []string{"--kubeconfig", `C:\Users\liqia\AppData\Local\Temp\tmp.X`, "get", "nodes"},
		},
		{
			in:   []string{"--kubeconfig=/c/Users/liqia/AppData/Local/Temp/tmp.X", "get", "nodes"},
			want: []string{`--kubeconfig=C:\Users\liqia\AppData\Local\Temp\tmp.X`, "get", "nodes"},
		},
		{
			in:   []string{"apply", "-f", "/c/manifests/app.yaml"},
			want: []string{"apply", "-f", `C:\manifests\app.yaml`},
		},
	}
	for _, tc := range cases {
		got := pathconv.NativeArgsIn(drives, tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("NativeArgsIn(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
