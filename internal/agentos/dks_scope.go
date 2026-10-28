package agentos

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/qiangli/yoke/external/kube"
	"github.com/qiangli/yoke/pkg/dagrun"
)

var scopeSegment = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func dksScopeNamespace(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("scope path is empty")
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if !scopeSegment.MatchString(part) {
			return "", fmt.Errorf("invalid scope path %q", path)
		}
	}
	ns := "scope-" + strings.Join(parts, "-")
	if len(ns) > 63 {
		return "", fmt.Errorf("scope namespace exceeds 63 characters")
	}
	return ns, nil
}

// dksScopeConfig uses the same kubeconfig selection as bashy kubectl and helm.
// Namespace changes require one writable file; a multi-file KUBECONFIG would
// otherwise make the destination of the edit ambiguous.
func dksScopeConfig() (string, *clientcmdapi.Config, error) {
	res, err := kube.ResolveKubeconfig()
	if err != nil {
		return "", nil, err
	}
	path := res.KUBECONFIG
	if explicit := os.Getenv("KUBECONFIG"); explicit != "" {
		path = explicit
	}
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil, err
		}
		path = filepath.Join(home, ".kube", "config")
	}
	if strings.Contains(path, string(os.PathListSeparator)) {
		return "", nil, fmt.Errorf("multiple kubeconfig files are not supported for namespace selection")
	}
	config, err := clientcmd.LoadFromFile(path)
	if err != nil {
		return "", nil, err
	}
	if config.CurrentContext == "" || config.Contexts[config.CurrentContext] == nil {
		return "", nil, fmt.Errorf("kubeconfig has no current context")
	}
	return path, config, nil
}

type dksNamespaceClient interface {
	List(context.Context, metav1.ListOptions) (*corev1.NamespaceList, error)
	Access(context.Context, string) (bool, error)
}

type liveDKSNamespaceClient struct{ client kubernetes.Interface }

func (c liveDKSNamespaceClient) List(ctx context.Context, opts metav1.ListOptions) (*corev1.NamespaceList, error) {
	return c.client.CoreV1().Namespaces().List(ctx, opts)
}
func (c liveDKSNamespaceClient) Access(ctx context.Context, ns string) (bool, error) {
	review, err := c.client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authv1.ResourceAttributes{Namespace: ns, Verb: "get", Group: "", Resource: "pods"}},
	}, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	if review.Status.EvaluationError != "" {
		return false, fmt.Errorf("access review for %s: %s", ns, review.Status.EvaluationError)
	}
	return review.Status.Allowed, nil
}

type dksNamespaceRow struct {
	Namespace string `json:"namespace"`
	Current   bool   `json:"current"`
}

func dksScopeRows(ctx context.Context, client dksNamespaceClient, current string) ([]dksNamespaceRow, error) {
	list, err := client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rows := make([]dksNamespaceRow, 0)
	for _, item := range list.Items {
		if !strings.HasPrefix(item.Name, "scope-") {
			continue
		}
		allowed, err := client.Access(ctx, item.Name)
		if err != nil {
			return nil, err
		}
		if allowed {
			rows = append(rows, dksNamespaceRow{Namespace: item.Name, Current: item.Name == current})
		}
	}
	return rows, nil
}

func dksScopeStore() (*dagrun.Store, error) {
	home := strings.TrimSpace(os.Getenv("BASHY_HOME"))
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(home, ".bashy")
	}
	return dagrun.OpenStore(filepath.Join(home, "dks", "context"))
}

// handleDKSScope extends the existing use verb and adds its namespace list.
// Other use forms keep going to the existing workspace handler.
func handleDKSScope(args []string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	if args[0] == "use" {
		if len(args) == 1 {
			store, err := dksScopeStore()
			if err == nil {
				_, selected, currentErr := store.Current()
				if currentErr == nil && selected {
					return false, 0
				}
			}
			_, cfg, err := dksScopeConfig()
			if err == nil {
				ns := cfg.Contexts[cfg.CurrentContext].Namespace
				if strings.HasPrefix(ns, "scope-") {
					fmt.Printf("dks: scope namespace %s\n", ns)
					return true, 0
				}
			}
			return false, 0
		}
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			fmt.Println("usage: bashy dks use @scope:<path> | <name>[@k8s:<context>] | @ws:<id> | --none\n@scope:<path> selects the matching scope namespace in the current kubeconfig context.")
			return true, 0
		}
		if len(args) == 2 && strings.HasPrefix(args[1], "@scope:") {
			ns, err := dksScopeNamespace(strings.TrimPrefix(args[1], "@scope:"))
			if err == nil {
				path, cfg, loadErr := dksScopeConfig()
				if loadErr != nil {
					err = loadErr
				} else {
					cfg.Contexts[cfg.CurrentContext].Namespace = ns
					err = clientcmd.WriteToFile(*cfg, path)
				}
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "bashy dks use: %v\n", err)
				return true, 1
			}
			store, err := dksScopeStore()
			if err != nil {
				fmt.Fprintf(os.Stderr, "bashy dks use: %v\n", err)
				return true, 1
			}
			if err := store.Clear(); err != nil {
				fmt.Fprintf(os.Stderr, "bashy dks use: %v\n", err)
				return true, 1
			}
			fmt.Printf("dks: using %s\n", ns)
			return true, 0
		}
		return false, 0
	}
	if args[0] != "list" {
		return false, 0
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Println("usage: bashy dks list [--json]\nList scope namespaces reachable by the issued kubeconfig account.")
		return true, 0
	}
	if len(args) > 2 || len(args) == 2 && args[1] != "--json" {
		fmt.Fprintln(os.Stderr, "usage: bashy dks list [--json]")
		return true, 2
	}
	path, cfg, err := dksScopeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks list: %v\n", err)
		return true, 1
	}
	rest, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks list: %v\n", err)
		return true, 1
	}
	client, err := kubernetes.NewForConfig(rest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks list: %v\n", err)
		return true, 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := dksScopeRows(ctx, liveDKSNamespaceClient{client}, cfg.Contexts[cfg.CurrentContext].Namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy dks list: %v\n", err)
		return true, 1
	}
	if len(args) == 2 {
		_ = json.NewEncoder(os.Stdout).Encode(rows)
	} else {
		for _, row := range rows {
			marker := " "
			if row.Current {
				marker = "*"
			}
			fmt.Printf("%s %s\n", marker, row.Namespace)
		}
	}
	return true, 0
}
