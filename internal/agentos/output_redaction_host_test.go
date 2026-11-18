package agentos

import (
	"bytes"
	"github.com/qiangli/yoke/pkg/secrets"
	"github.com/qiangli/yoke/pkg/service"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellOutputCaptureMasksMintedAndContextCredentials(t *testing.T) {
	isolateOutputReduction(t)
	t.Setenv("BASHY_OUTPUT_REDUCE", "off")
	minted := "synthetic-agent-output-minted-value"
	if err := secrets.RegisterHostSecret("AGENT_OUTPUT_TEST", minted); err != nil {
		t.Fatal(err)
	}
	token := "9f42e8a1b0c7d6358a4f1c962e0b3d75f8a9c6102e4d7b953a6f8c012d35b7e4"
	fixture := fixtureExec(map[string][2]string{"go test": {"ready " + minted + " token=" + token + "\n", "Authorization: Bearer " + token + "\n"}}, nil)
	out, errOut, err := runReducedSource(t, "go test", false, fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{minted, token} {
		if strings.Contains(out, value) || strings.Contains(errOut, value) {
			t.Fatal("credential survived captured agent command output")
		}
	}
	if !strings.Contains(out, "ready") || !strings.Contains(out+errOut, "[redacted:") {
		t.Fatal("captured output lost ordinary content or redaction marker")
	}
}

func TestProcessProviderRedactsChildUnitLog(t *testing.T) {
	t.Setenv("BASHY_SUPERVISORD_TEST_CHILD", "redaction-fixture")
	root := t.TempDir()
	p := newProcessProvider(os.Args[0], root, nil)
	unit := testUnit("redaction", true)
	unit.Exec = []string{os.Args[0], "redaction"}
	if err := p.Activate(t.Context(), unit, service.Runtime{}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "redaction child exits", func() bool { return p.Status(unit.Name).State == service.Inactive })
	if err := p.Deactivate(t.Context(), unit.Name); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, unit.Name, unitLogName))
	if err != nil {
		t.Fatal(err)
	}
	token := "9f42e8a1b0c7d6358a4f1c962e0b3d75f8a9c6102e4d7b953a6f8c012d35b7e4"
	if bytes.Contains(data, []byte(token)) {
		t.Fatal("child credential leaked to supervisord unit log")
	}
	if !bytes.Contains(data, []byte("ready")) || !bytes.Contains(data, []byte("[redacted:")) {
		t.Fatal("redacted child log missing expected ordinary output")
	}
}
