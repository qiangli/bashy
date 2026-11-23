package desk

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// installWrappedForkSkip names shaped example apps whose verification checks
// require a network `<app> install` step to clone and build upstream web
// assets before `bashy src pr verify` can run. Every entry skips explicitly
// with its reason and plan-appendix reference — never a silent omission.
var installWrappedForkSkip = map[string]string{
	"whiteboard": "network install required (`whiteboard install` clones and builds the pinned Excalidraw bundle checked by checks/whiteboard.bsh); see docs/plan-s682-consumer-shaping-example-apps.md#appendix-install-wrapped-example-apps",
	"board":      "network install required (`board install` clones and builds the pinned Focalboard bundle embedded by board-console); see docs/plan-s682-consumer-shaping-example-apps.md#appendix-install-wrapped-example-apps",
	"table":      "network install required and Linux-only (`table install` clones and builds the pinned Mathesar bundle); see docs/plan-s682-consumer-shaping-example-apps.md#appendix-install-wrapped-example-apps",
}

func TestInstallWrappedSkipReasonsAreExplicit(t *testing.T) {
	for _, name := range []string{"whiteboard", "board", "table"} {
		reason := installWrappedForkSkip[name]
		if !strings.Contains(reason, "network install required") ||
			!strings.Contains(reason, "docs/plan-s682-consumer-shaping-example-apps.md#appendix-install-wrapped-example-apps") {
			t.Fatalf("%s skip reason %q must state network install requirement and plan appendix pointer", name, reason)
		}
	}
}

// TestDeskToolsForkAndRollback is the shaped fork proof per desk tool, driven
// through the real CLI: a principal forks the tool, the verifier passes the
// fork against the contract the tool ships, the gate activates it, and undo
// returns the principal to the revision it ran before. A tool that ships no
// base contract (no effect cap, no checks/) fails at verify.
func TestDeskToolsForkAndRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("builds bashy, and each verify provisions the Go toolchain in the clone")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the fork proof drives the unix product build")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required to commit the fork")
	}
	for _, name := range []string{"board", "table"} {
		t.Run(name, func(t *testing.T) {
			t.Skip(installWrappedForkSkip[name])
		})
	}
	for name, root := range shapedTools {
		t.Run(name, func(t *testing.T) {
			if reason, ok := installWrappedForkSkip[name]; ok {
				t.Skip(reason)
			}
			bashy := productBashy(t)
			t.Parallel()
			src, err := filepath.Abs(root)
			if err != nil {
				t.Fatal(err)
			}
			f := newForkHome(t, bashy, gitBin)
			f.bashy("app", "add", name, "--type", "shaped", "--source", src)
			repo := filepath.Join(f.home, "src", name)

			prev := f.fork(name, repo, "alice-fork", "first")
			f.bashy("src", "activate", name, prev, "--principal", alice)
			if got := f.active(name); got != prev {
				t.Fatalf("after activate: active %s, want %s", got, prev)
			}
			// The next fork is cut from the revision alice runs, so its
			// verdict is bound to that base.
			f.bashy("src", "pr", "merge", f.lastPR)

			next := f.fork(name, repo, "alice-fork-2", "second")
			f.bashy("src", "activate", name, next, "--principal", alice)
			if got := f.active(name); got != next {
				t.Fatalf("after second activate: active %s, want %s", got, next)
			}

			f.bashy("src", "undo", name, "--principal", alice)
			if got := f.active(name); got != prev {
				t.Fatalf("after undo: active %s, want the previous revision %s", got, prev)
			}
		})
	}
}

const alice = "device:alice"

var (
	productOnce sync.Once
	productBin  string
	productErr  string
)

// productBashy builds the candidate bashy once through the product wrapper.
// The binary must be named bashy: the verifier runs checks under its own
// executable only when it is bashy.
func productBashy(t *testing.T) string {
	t.Helper()
	productOnce.Do(func() {
		dir, err := os.MkdirTemp("", "desk-fork-bashy-")
		if err != nil {
			productErr = err.Error()
			return
		}
		bin := filepath.Join(dir, "bashy")
		cmd := exec.Command("./scripts/go-product.sh", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = filepath.Join("..", "..")
		if out, err := cmd.CombinedOutput(); err != nil {
			productErr = err.Error() + "\n" + string(out)
			return
		}
		productBin = bin
	})
	if productErr != "" {
		t.Fatalf("build bashy: %s", productErr)
	}
	return productBin
}

// forkHome is one isolated bashy home: BASHY_HOME, HOME and XDG_CONFIG_HOME
// all live in the test's temp dir, so app add never touches the host's
// console registry.
type forkHome struct {
	t      *testing.T
	bin    string
	git    string
	home   string
	env    []string
	lastPR string
}

func newForkHome(t *testing.T, bin, git string) *forkHome {
	root := t.TempDir()
	// Keep the host Go caches so an isolated HOME neither re-downloads modules
	// nor leaves a read-only module cache behind. The fork's required checks
	// build the clone alone, so they need the shared desk modules from a
	// published local proxy (BASHY_GOMOD_PROXY), exactly as a registered host
	// has them after register-modules.sh.
	caches, err := exec.Command("go", "env", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	goenv := strings.Split(strings.TrimSpace(string(caches)), "\n")
	home := filepath.Join(root, "bashy")
	env := append(os.Environ(),
		"BASHY_HOME="+home,
		"HOME="+root,
		"XDG_CONFIG_HOME="+filepath.Join(root, ".config"),
		"BASHY_PRINCIPAL="+alice,
		"BASHY_GOMOD_PROXY="+deskProxy(t),
		"GOMODCACHE="+goenv[0], "GOCACHE="+goenv[1],
		"GIT_AUTHOR_NAME=alice", "GIT_AUTHOR_EMAIL=alice@example.invalid",
		"GIT_COMMITTER_NAME=alice", "GIT_COMMITTER_EMAIL=alice@example.invalid",
	)
	return &forkHome{t: t, bin: bin, git: git, home: home, env: env}
}

func (f *forkHome) run(dir, bin string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("%s %s: %v\n%s", filepath.Base(bin), strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (f *forkHome) bashy(args ...string) string { return f.run(filepath.Dir(f.home), f.bin, args...) }

var openedPR = regexp.MustCompile(`opened PR (\S+):`)

// fork commits a trivial change on a topic branch off local, opens a PR for
// it and verifies it for alice. It returns the verified head revision.
func (f *forkHome) fork(app, repo, branch, line string) string {
	f.t.Helper()
	f.bashy("src", "branch", app, branch)
	file := filepath.Join(repo, "FORK.txt")
	body, _ := os.ReadFile(file)
	if err := os.WriteFile(file, append(body, line+"\n"...), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.run(repo, f.git, "add", "FORK.txt")
	f.run(repo, f.git, "commit", "-q", "-m", "fork: "+line)
	head := strings.TrimSpace(f.run(repo, f.git, "rev-parse", "HEAD"))

	m := openedPR.FindStringSubmatch(f.bashy("src", "pr", "open", app, branch, "--title", line))
	if m == nil {
		f.t.Fatalf("src pr open %s: no PR id reported", branch)
	}
	f.lastPR = m[1]
	out := f.bashy("src", "pr", "verify", f.lastPR)
	if !strings.Contains(out, "verdict: pass") {
		f.t.Fatalf("src pr verify %s: want verdict pass\n%s", f.lastPR, out)
	}
	return head
}

// active is alice's active revision of app.
func (f *forkHome) active(app string) string {
	f.t.Helper()
	var h struct {
		Active string `json:"active"`
	}
	out := f.bashy("src", "history", app, "--json")
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		f.t.Fatalf("src history --json: %v\n%s", err, out)
	}
	return h.Active
}
