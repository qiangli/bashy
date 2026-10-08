package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep the browserless app-service proof on every native CI runner. In
// particular, the workflow used to run these tests only by hand on Linux and
// macOS, leaving the shipped Windows service path unmeasured.
func TestCIExercisesAppServiceE2EOnEveryOS(t *testing.T) {
	workflow := filepath.Join("..", "..", ".github", "workflows", "test.yml")
	b, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, testName := range []string{"TestE2ERegisteredAppFlow", "TestE2ECustomLauncher"} {
		if !strings.Contains(text, testName) {
			t.Errorf("%s does not run %s", workflow, testName)
		}
	}
}
