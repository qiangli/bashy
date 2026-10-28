package agentos

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

type fakeDKSNamespaceClient struct {
	items   []string
	allowed map[string]bool
	seen    []string
}

func (f *fakeDKSNamespaceClient) List(context.Context, metav1.ListOptions) (*corev1.NamespaceList, error) {
	list := &corev1.NamespaceList{}
	for _, name := range f.items {
		list.Items = append(list.Items, corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
	return list, nil
}
func (f *fakeDKSNamespaceClient) Access(_ context.Context, name string) (bool, error) {
	f.seen = append(f.seen, name)
	return f.allowed[name], nil
}

func TestDKSScopeNamespace(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"grade-10-9/a", "scope-grade-10-9-a"},
		{"a", "scope-a"},
		{"", ""}, {"a//b", ""}, {"A/b", ""}, {"a/../b", ""},
	} {
		got, err := dksScopeNamespace(tc.path)
		if tc.want == "" && err == nil || tc.want != "" && (err != nil || got != tc.want) {
			t.Errorf("%q = %q, %v; want %q", tc.path, got, err, tc.want)
		}
	}
}

func TestDKSScopeUseWritesCurrentContext(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config")
	cfg := api.NewConfig()
	cfg.CurrentContext = "satellite"
	cfg.Contexts["satellite"] = &api.Context{Cluster: "satellite", AuthInfo: "member", Namespace: "default"}
	cfg.Contexts["other"] = &api.Context{Cluster: "other", AuthInfo: "member", Namespace: "unchanged"}
	cfg.Clusters["satellite"] = &api.Cluster{Server: "https://example.invalid"}
	cfg.AuthInfos["member"] = &api.AuthInfo{Token: "fake"}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)
	handled, code := handleDKSScope([]string{"use", "@scope:grade-10-9/a"})
	if !handled || code != 0 {
		t.Fatalf("handled=%v code=%d", handled, code)
	}
	got, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contexts["satellite"].Namespace != "scope-grade-10-9-a" {
		t.Errorf("namespace = %q", got.Contexts["satellite"].Namespace)
	}
	if got.Contexts["other"].Namespace != "unchanged" {
		t.Errorf("other context changed")
	}
	if handled, _ := handleDKSScope([]string{"use", "workspace"}); handled {
		t.Error("workspace selection was intercepted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestDKSScopeRowsFiltersAccess(t *testing.T) {
	f := &fakeDKSNamespaceClient{items: []string{"default", "scope-grade-10-9-a", "scope-secret", "scope-other"}, allowed: map[string]bool{"scope-grade-10-9-a": true, "scope-other": true}}
	rows, err := dksScopeRows(context.Background(), f, "scope-grade-10-9-a")
	if err != nil {
		t.Fatal(err)
	}
	want := []dksNamespaceRow{{Namespace: "scope-grade-10-9-a", Current: true}, {Namespace: "scope-other"}}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %#v; want %#v", rows, want)
	}
	if !reflect.DeepEqual(f.seen, []string{"scope-grade-10-9-a", "scope-secret", "scope-other"}) {
		t.Errorf("access checks = %v", f.seen)
	}
}
