package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/qiangli/bash/pathconv"
)

func TestKubectlPathAndEnvConversion(t *testing.T) {
	// Verify environment variable conversion for KUBECONFIG and related variables.
	t.Setenv("KUBECONFIG", "/c/Users/user/AppData/Local/Temp/tmp.X")
	t.Setenv("OUTPOST_KUBECONFIG_PATH", "/c/Users/user/.kube/outpost.yaml")
	t.Setenv("DKS_PEER_KUBECONFIG", "/c/Users/user/.kube/peer.yaml")
	t.Setenv("DKS_FEDERATION_KUBECONFIG", "/c/Users/user/.kube/fed.yaml")

	prepareKubeEnvironment("windows")

	if got := os.Getenv("KUBECONFIG"); got != `C:\Users\user\AppData\Local\Temp\tmp.X` {
		t.Errorf("KUBECONFIG = %q, want C:\\Users\\user\\AppData\\Local\\Temp\\tmp.X", got)
	}
	if got := os.Getenv("OUTPOST_KUBECONFIG_PATH"); got != `C:\Users\user\.kube\outpost.yaml` {
		t.Errorf("OUTPOST_KUBECONFIG_PATH = %q, want C:\\Users\\user\\.kube\\outpost.yaml", got)
	}
	if got := os.Getenv("DKS_PEER_KUBECONFIG"); got != `C:\Users\user\.kube\peer.yaml` {
		t.Errorf("DKS_PEER_KUBECONFIG = %q, want C:\\Users\\user\\.kube\\peer.yaml", got)
	}
	if got := os.Getenv("DKS_FEDERATION_KUBECONFIG"); got != `C:\Users\user\.kube\fed.yaml` {
		t.Errorf("DKS_FEDERATION_KUBECONFIG = %q, want C:\\Users\\user\\.kube\\fed.yaml", got)
	}

	// Verify argument conversion for kubectl / helm command line flags.
	drives := pathconv.DrivesOf("C")
	cases := []struct {
		in   []string
		want []string
	}{
		{
			in:   []string{"--kubeconfig", "/c/Users/user/AppData/Local/Temp/tmp.X", "get", "nodes"},
			want: []string{"--kubeconfig", `C:\Users\user\AppData\Local\Temp\tmp.X`, "get", "nodes"},
		},
		{
			in:   []string{"--kubeconfig=/c/Users/user/AppData/Local/Temp/tmp.X", "get", "nodes"},
			want: []string{`--kubeconfig=C:\Users\user\AppData\Local\Temp\tmp.X`, "get", "nodes"},
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

func TestKubectlPreservesNonWindowsEnvironment(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			t.Setenv("KUBECONFIG", "/c/config:/d/config")
			prepareKubeEnvironment(goos)
			if got := os.Getenv("KUBECONFIG"); got != "/c/config:/d/config" {
				t.Fatalf("changed non-Windows env: %q", got)
			}
		})
	}
}

func TestKubectlWindowsEnvironmentPathList(t *testing.T) {
	t.Setenv("KUBECONFIG", "/c/config:/d/config")
	t.Setenv("OUTPOST_KUBECONFIG_PATH", "")
	prepareKubeEnvironment("windows")
	if got := os.Getenv("KUBECONFIG"); got != `C:\config;D:\config` {
		t.Fatalf("path list = %q", got)
	}
	if got := os.Getenv("OUTPOST_KUBECONFIG_PATH"); got != "" {
		t.Fatalf("empty env = %q", got)
	}
}
