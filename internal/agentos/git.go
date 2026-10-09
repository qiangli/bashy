package agentos

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	outgit "github.com/qiangli/yoke/git"
)

// bashy git … — embedded git client. Pure-Go go-git backend so the
// command works without a system `git` binary, which is the load-
// bearing case on Windows. Scope is the development cycle (clone →
// edit → add → commit → push), the read/inspect verbs (status, diff,
// log, branch, show, remote, fetch, merge-base, rev-list, ls-files,
// blame, grep), and local writes (merge ff, tag, reset, rm, config).
// Conflict-resolution machinery — rebase, submodules, reflog,
// bisect — is intentionally out of scope: a half-applied replay is
// worse than no support. The engine additionally serves cherry,
// revert, stash, worktree, clean and apply (sprint 252), dispatched
// automatically for verbs without a dedicated subcommand.
//
// `bashy git ...` always resolves to the native implementation by
// default; only --external permits the host binary as a fallback,
// so the default path stays hermetic with or without system git.

func init() {
	// Hint messages inside the shared git package ("run \"<CLIName>
	// config user.name ...\"") should name our spelling of the command.
	outgit.CLIName = "bashy git"
}

func gitCmd() *cobra.Command {
	var external bool
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Embedded git client (clone, pull, status, commit, push, …)",
		Long: `bashy git is a small, self-contained git client for the typical
development cycle: clone → edit → add → commit → push, the common
read paths (status, diff, log, branch, show, remote, fetch,
merge-base, rev-list, ls-files, blame, grep), and local writes
(merge fast-forward, tag, reset, rm, config).

It is implemented on top of go-git/v5 and does NOT require a system
'git' binary — that's the point: bashy stays self-sufficient on
Windows machines where setting up real git is painful. Local-path
remotes are served in-process too.

pull and merge integrate fast-forwards only; local changes that don't
conflict with the incoming commits survive, like real git. Diverged
histories are an error — bashy git does not do conflict resolution.
Beyond the verbs below, the native engine also serves cherry, revert,
stash, worktree, clean and apply (dispatched automatically); still
out of scope (use --external where a host git exists): rebase,
bisect, submodules, reflog and the other verbs named below.

--external permits the host git binary as a fallback: verbs the
native engine cannot serve run against system git with the verbatim
arguments (stdout/stderr stream through; a missing binary fails
loudly). Without the flag, unserved verbs fail with a workaround
hint instead. Built-in verbs always run native.

Authentication for HTTPS remotes uses --username/--password when
supplied, otherwise falls back to $GITHUB_TOKEN or $GIT_TOKEN as the
basic-auth password (with user "oauth2", which GitHub accepts).`,
	}
	cmd.PersistentFlags().BoolVar(&external, "external", false, "allow falling back to the host git binary for verbs the native engine cannot serve (native remains the default)")
	cmd.AddCommand(
		gitCloneCmd(),
		gitInitCmd(),
		gitAddCmd(),
		gitCommitCmd(),
		gitStatusCmd(),
		gitLogCmd(),
		gitPushCmd(),
		gitPullCmd(),
		gitFetchCmd(),
		gitBranchCmd(),
		gitCheckoutCmd(),
		gitDiffCmd(),
		gitRemoteCmd(),
		gitShowCmd(),
		gitRevParseCmd(),
		gitMergeCmd(),
		gitMergeBaseCmd(),
		gitRevListCmd(),
		gitConfigCmd(),
		gitTagCmd(),
		gitResetCmd(),
		gitRmCmd(),
		gitLsFilesCmd(),
		gitBlameCmd(),
		gitGrepCmd(),
	)
	// Runtime git errors (diverged pull, dirty merge, bad revision …)
	// are conditions, not usage mistakes — don't dump help text after
	// them. Same convention as connect/ssh/sshd.
	cmd.SilenceUsage = true
	for _, sub := range cmd.Commands() {
		sub.SilenceUsage = true
	}
	// Unmatched subcommands land here and dispatch through the native
	// engine; only --external permits the host binary as a fallback.
	// Verbs neither serves get a workaround hint instead of cobra's
	// generic "unknown command".
	cmd.Args = cobra.ArbitraryArgs
	// Let flags meant for an unimplemented verb (e.g. `rebase -i`)
	// reach the handler below instead of dying on flag parsing.
	cmd.FParseErrWhitelist = cobra.FParseErrWhitelist{UnknownFlags: true}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		var res *outgit.ExecResult
		var err error
		if external {
			res, err = outgit.RunExternal(cmd.Context(), ".", args)
		} else {
			res, err = outgit.Exec(cmd.Context(), ".", args)
		}
		if err == nil {
			return renderGitResult(cmd, res)
		}
		if !errors.Is(err, outgit.ErrUnsupported) {
			return err
		}
		verb := args[0]
		if hint, ok := unimplementedGitVerbs[verb]; ok {
			if external {
				return fmt.Errorf("git %s is not served by the native engine, and no host git binary took it.\n%s", verb, hint)
			}
			return fmt.Errorf("git %s is not served by bashy's native git engine.\n%s\n(retry with --external where a host git binary exists)", verb, hint)
		}
		return fmt.Errorf("unknown git subcommand %q — see \"bashy git --help\" for the supported set", verb)
	}
	return cmd
}

// splitGitExternal extracts the one-door selector from a `bashy git`
// argv: `--external` (or --external=true/false in any position) is
// consumed, everything else passes through verbatim. A malformed
// --external= value fails loudly; it is never forwarded to real git,
// where it would die as "unknown option".
func splitGitExternal(args []string) (external bool, rest []string, err error) {
	for _, a := range args {
		if a == "--external" {
			external = true
			continue
		}
		if v, ok := strings.CutPrefix(a, "--external="); ok {
			b, perr := strconv.ParseBool(v)
			if perr != nil {
				return false, nil, fmt.Errorf("invalid --external value %q: want true or false", v)
			}
			external = b
			continue
		}
		rest = append(rest, a)
	}
	return external, rest, nil
}

// gitExitError carries a host-faithful exit code (128 fatal, 1
// no-match/quiet-dirty, ...) from the native door to the front-door
// dispatch, which maps it to the process status. The message is the
// same text the plain error carried, so string assertions are unaffected.
type gitExitError struct {
	code int
	msg  string
}

func (e *gitExitError) Error() string { return e.msg }

// renderGitResult streams an engine result to the command outputs. A
// non-zero exit becomes a typed error (the CLI's usual error path, like
// every other verb's typed error); stdout/stderr already streamed through.
func renderGitResult(cmd *cobra.Command, res *outgit.ExecResult) error {
	if res.Stdout != "" {
		fmt.Fprint(cmd.OutOrStdout(), res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(cmd.ErrOrStderr(), res.Stderr)
	}
	if res.ExitCode != 0 {
		return &gitExitError{code: res.ExitCode, msg: fmt.Sprintf("git: exit status %d", res.ExitCode)}
	}
	return nil
}

// unimplementedGitVerbs maps git verbs the native engine cannot serve
// to a workaround hint. Verbs the engine HAS started serving (cherry,
// revert, stash, worktree, clean, apply) are dispatched before this map
// is consulted; their entries below cover only the flag combinations
// the engine still rejects. Keep in sync with yoke/git/GAPS.md.
var unimplementedGitVerbs = map[string]string{
	"rebase":      "it needs conflict resolution. Bring your branch up to date with \"bashy git merge <base>\" (fast-forward), or recreate it: checkout the base, \"checkout -b\" a fresh branch, and re-apply your changes",
	"cherry-pick": "linear conflict-free picks run natively; --continue/--abort/--skip and conflicts need a host git (--external) or a by-hand re-apply (\"bashy git show <commit>\" prints the patch)",
	"revert":      "single-commit reverts run natively; merges and sequencer flags need a host git (--external) or a by-hand inversion (\"bashy git show <commit>\" prints the patch)",
	"stash":       "push/pop/list run natively; drop/apply/show and --index/-u need a host git (--external), or commit to a temporary branch instead",
	"apply":       "unified patches apply natively; binary patches and -R/--cached/--index need a host git (--external)",
	"am":          "mailbox patch application is out of scope. Make the edits directly and commit",
	"clean":       "clean -fd/-n runs natively; -x/-X/-e/-i need a host git (--external)",
	"submodule":   "submodules are out of scope. Clone each submodule repo separately with \"bashy git clone\"",
	"worktree":    "add/remove/list run natively; exotic flags need a host git (--external)",
	"reflog":      "go-git does not maintain a reflog. \"bashy git log\" shows reachable history",
	"bisect":      "bisect is out of scope. \"bashy git checkout <commit>\" lets you test revisions manually",
	"mergetool":   "interactive conflict resolution is out of scope",
	"gc":          "object-store maintenance is unnecessary here — go-git repos work without it",
	"fsck":        "object-store verification is out of scope",
	"archive":     "archive export is out of scope. \"bashy git checkout <rev>\" then copy the tree",
}

func gitCloneCmd() *cobra.Command {
	var (
		depth        int
		branch       string
		singleBranch bool
		username     string
		password     string
		sshKey       string
		sshKeyPass   string
		quiet        bool
	)
	cmd := &cobra.Command{
		Use:   "clone <url> [directory]",
		Short: "Clone a repository",
		Args:  cobra.RangeArgs(1, 2),
		Example: `  bashy git clone https://github.com/user/repo.git
  bashy git clone https://github.com/user/repo.git ./my-project
  bashy git clone https://github.com/user/repo.git --depth 1
  bashy git clone https://github.com/user/repo.git --branch v1.2.3 --single-branch
  GITHUB_TOKEN=ghp_xxx bashy git clone https://github.com/org/private.git`,
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			var path string
			if len(args) > 1 {
				path = args[1]
			}
			var progress io.Writer = cmd.ErrOrStderr()
			if quiet {
				progress = nil
			}
			result, err := outgit.Clone(outgit.CloneOptions{
				URL:          url,
				Path:         path,
				Depth:        depth,
				Branch:       branch,
				SingleBranch: singleBranch,
				Auth: outgit.AuthConfig{
					Username:   username,
					Password:   password,
					SSHKey:     sshKey,
					SSHKeyPass: sshKeyPass,
				},
				Progress: progress,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 0, "Shallow clone with depth")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Branch or tag to check out (instead of remote HEAD)")
	cmd.Flags().BoolVar(&singleBranch, "single-branch", false, "When --branch is set, fetch only that ref")
	cmd.Flags().StringVar(&username, "username", "", "Username for HTTPS basic auth")
	cmd.Flags().StringVar(&password, "password", "", "Password/token for HTTPS basic auth")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key (use with ssh:// URLs)")
	cmd.Flags().StringVar(&sshKeyPass, "ssh-key-pass", "", "Passphrase for --ssh-key (if encrypted)")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress progress output")
	return cmd
}

func gitInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Initialize a git repository",
		Args:  cobra.MaximumNArgs(1),
		Example: `  bashy git init
  bashy git init ./my-project`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			result, err := outgit.Init(outgit.InitOptions{Path: path})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
}

func gitAddCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "add <path>...",
		Short: "Add files to the staging area",
		Args:  cobra.MinimumNArgs(0),
		Example: `  bashy git add file.txt
  bashy git add .
  bashy git add -A`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if all || (len(args) == 1 && args[0] == ".") {
				result, err := outgit.Add(outgit.AddOptions{RepoPath: repo, All: true})
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), result.Message)
				return nil
			}
			if len(args) == 0 {
				return fmt.Errorf("nothing to add — pass a path or -A")
			}
			for _, p := range args {
				result, err := outgit.Add(outgit.AddOptions{RepoPath: repo, Path: p})
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "A", false, "Stage all changes")
	return cmd
}

func gitCommitCmd() *cobra.Command {
	var (
		message     string
		amend       bool
		all         bool
		authorName  string
		authorEmail string
	)
	cmd := &cobra.Command{
		Use:   "commit [message]",
		Short: "Record staged changes as a new commit",
		Args:  cobra.MaximumNArgs(1),
		Example: `  bashy git commit -m "Initial commit"
  bashy git commit -a -m "Fix bug"        # stage tracked + commit
  bashy git commit --amend -m "Reword"
  bashy git commit --author-name "Alice" --author-email "alice@example.com" -m "..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if message == "" && len(args) > 0 {
				message = args[0]
			}
			result, err := outgit.Commit(outgit.CommitOptions{
				Message:     message,
				Amend:       amend,
				All:         all,
				AuthorName:  authorName,
				AuthorEmail: authorEmail,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "Commit message")
	cmd.Flags().BoolVar(&amend, "amend", false, "Amend the previous commit")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "Stage tracked-and-modified files before committing")
	cmd.Flags().StringVar(&authorName, "author-name", "", "Override author name (otherwise read from repo/global config)")
	cmd.Flags().StringVar(&authorEmail, "author-email", "", "Override author email")
	return cmd
}

func gitStatusCmd() *cobra.Command {
	var short, porcelain bool
	cmd := &cobra.Command{
		Use:     "status [path]",
		Short:   "Show working tree status",
		Args:    cobra.MaximumNArgs(1),
		Example: `  bashy git status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if short || porcelain {
				// Porcelain short form is served by the engine
				// (nativeStatus --short); other status flags stay
				// loud cobra errors, never silent approximations.
				res, err := outgit.Exec(cmd.Context(), ".", []string{"status", "--short"})
				if err != nil {
					return err
				}
				return renderGitResult(cmd, res)
			}
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			result, entries, err := outgit.Status(repo)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if entries == nil {
				fmt.Fprintln(out, result.Message)
				return nil
			}
			var unstaged, staged, untracked []outgit.StatusEntry
			for _, e := range entries {
				switch {
				case e.Status == "??":
					untracked = append(untracked, e)
				case e.Staged:
					staged = append(staged, e)
				default:
					unstaged = append(unstaged, e)
				}
			}
			section := func(title string, list []outgit.StatusEntry, first bool) bool {
				if len(list) == 0 {
					return first
				}
				if !first {
					fmt.Fprintln(out)
				}
				fmt.Fprintln(out, title)
				for _, e := range list {
					fmt.Fprintf(out, "  %s  %s\n", e.Status, e.File)
				}
				return false
			}
			first := section("Changes to be committed:", staged, true)
			first = section("Changes not staged for commit:", unstaged, first)
			section("Untracked files:", untracked, first)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&short, "short", "s", false, "Porcelain short format (engine-native)")
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Alias for --short")
	return cmd
}

func gitLogCmd() *cobra.Command {
	var number int
	cmd := &cobra.Command{
		Use:   "log [path]",
		Short: "Show commit history (most recent first)",
		Args:  cobra.ArbitraryArgs,
		Example: `  bashy git log
  bashy git log -n 20
  bashy git log -1 --format=%h`,
		// Flags beyond -n (-1, --format, --oneline, …) belong to the engine's
		// log; parse by hand so they reach it instead of dying in cobra.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, a := range args {
				if a == "-h" || a == "--help" {
					return cmd.Help()
				}
			}
			if !legacyGitLogArgs(args) {
				res, err := outgit.Exec(cmd.Context(), ".", append([]string{"log"}, args...))
				if err != nil {
					if errors.Is(err, outgit.ErrUnsupported) {
						return fmt.Errorf("git log %s is not served by bashy's native git engine; retry with --external to run it on a host git", strings.Join(args, " "))
					}
					return err
				}
				return renderGitResult(cmd, res)
			}
			if err := cmd.Flags().Parse(args); err != nil {
				return err
			}
			args = cmd.Flags().Args()
			repo := "."
			if len(args) > 0 {
				repo = args[0]
			}
			_, entries, err := outgit.Log(outgit.LogOptions{RepoPath: repo, Number: number})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, e := range entries {
				fmt.Fprintf(out, "%s  %s\n", e.Hash, e.Message)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&number, "number", "n", 10, "Number of commits to show")
	return cmd
}

func gitPushCmd() *cobra.Command {
	var (
		force    bool
		username string
		password string
		sshKey   string
	)
	cmd := &cobra.Command{
		Use:   "push [remote] [branch]",
		Short: "Push the current branch to a remote",
		Args:  cobra.MaximumNArgs(2),
		Example: `  bashy git push
  bashy git push origin main
  bashy git push --force
  GITHUB_TOKEN=ghp_xxx bashy git push origin feature`,
		RunE: func(cmd *cobra.Command, args []string) error {
			remoteName := ""
			branch := ""
			if len(args) > 0 {
				remoteName = args[0]
			}
			if len(args) > 1 {
				branch = args[1]
			}
			result, err := outgit.Push(outgit.PushOptions{
				Remote: remoteName,
				Branch: branch,
				Force:  force,
				Auth: outgit.AuthConfig{
					Username: username,
					Password: password,
					SSHKey:   sshKey,
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force push (replaces remote history)")
	cmd.Flags().StringVar(&username, "username", "", "Username for HTTPS basic auth")
	cmd.Flags().StringVar(&password, "password", "", "Password/token for HTTPS basic auth")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key (ssh:// remotes)")
	return cmd
}

func gitPullCmd() *cobra.Command {
	var (
		username string
		password string
		sshKey   string
	)
	cmd := &cobra.Command{
		Use:   "pull [remote] [branch]",
		Short: "Fetch and integrate from a remote branch",
		Args:  cobra.MaximumNArgs(2),
		Example: `  bashy git pull
  bashy git pull origin main`,
		RunE: func(cmd *cobra.Command, args []string) error {
			remoteName := ""
			branch := ""
			if len(args) > 0 {
				remoteName = args[0]
			}
			if len(args) > 1 {
				branch = args[1]
			}
			result, err := outgit.Pull(outgit.PullOptions{
				Remote: remoteName,
				Branch: branch,
				Auth: outgit.AuthConfig{
					Username: username,
					Password: password,
					SSHKey:   sshKey,
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Username for HTTPS basic auth")
	cmd.Flags().StringVar(&password, "password", "", "Password/token for HTTPS basic auth")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key (ssh:// remotes)")
	return cmd
}

func gitFetchCmd() *cobra.Command {
	var (
		username string
		password string
		sshKey   string
		noTags   bool
	)
	cmd := &cobra.Command{
		Use:   "fetch [remote] [refspec...]",
		Short: "Download objects and refs from a remote",
		Args:  cobra.ArbitraryArgs,
		Example: `  bashy git fetch
  bashy git fetch origin
  bashy git fetch --no-tags ../workspace feature:feature`,
		RunE: func(cmd *cobra.Command, args []string) error {
			remoteName := ""
			if len(args) > 0 {
				remoteName = args[0]
			}
			var refspecs []string
			if len(args) > 1 {
				refspecs = args[1:]
			}
			result, err := outgit.Fetch(outgit.FetchOptions{
				Remote:   remoteName,
				RefSpecs: refspecs,
				NoTags:   noTags,
				Auth: outgit.AuthConfig{
					Username: username,
					Password: password,
					SSHKey:   sshKey,
				},
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Username for HTTPS basic auth")
	cmd.Flags().StringVar(&password, "password", "", "Password/token for HTTPS basic auth")
	cmd.Flags().StringVar(&sshKey, "ssh-key", "", "Path to SSH private key (ssh:// remotes)")
	cmd.Flags().BoolVar(&noTags, "no-tags", false, "Do not fetch tags")
	return cmd
}

func gitBranchCmd() *cobra.Command {
	var (
		del   bool
		force bool
	)
	cmd := &cobra.Command{
		Use:   "branch [name]",
		Short: "List, create, or delete branches",
		Args:  cobra.MaximumNArgs(1),
		Example: `  bashy git branch                     # list, marks current with *
  bashy git branch feature-x          # create
  bashy git branch -d old-branch      # delete`,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			result, branches, err := outgit.Branch(outgit.BranchOptions{
				Name:   name,
				Delete: del,
				Force:  force,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if branches != nil {
				for _, b := range branches {
					fmt.Fprintln(out, b)
				}
				return nil
			}
			fmt.Fprintln(out, result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&del, "delete", "d", false, "Delete the named branch")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force operation (with --delete)")
	return cmd
}

func gitCheckoutCmd() *cobra.Command {
	var create, force bool
	cmd := &cobra.Command{
		Use:   "checkout <branch>",
		Short: "Switch to a branch, create/reset it, or detach to a revision",
		Args:  cobra.ExactArgs(1),
		Example: `  bashy git checkout main
  bashy git checkout -b feature-x      # create and switch
  bashy git checkout -B feature-x      # create/reset and switch
  bashy git checkout <commit-sha>      # detached HEAD`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if create && force {
				return fmt.Errorf("checkout: -b and -B are mutually exclusive")
			}
			result, err := outgit.Checkout(outgit.CheckoutOptions{
				Branch: args[0],
				Create: create,
				Force:  force,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&create, "branch", "b", false, "Create the branch before switching")
	cmd.Flags().BoolVarP(&force, "force-branch", "B", false, "Create or reset the branch before switching")
	return cmd
}

func gitDiffCmd() *cobra.Command {
	var staged bool
	cmd := &cobra.Command{
		Use:   "diff [rev1 [rev2]]",
		Short: "Show changes (summary for the working tree, full patch between revisions)",
		Args:  cobra.MaximumNArgs(2),
		Example: `  bashy git diff                    # working-tree summary
  bashy git diff --staged
  bashy git diff HEAD~1 HEAD        # full patch between commits
  bashy git diff main..feature-x`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Revision arguments switch to commit-to-commit patch mode.
			if len(args) > 0 {
				revA, revB := args[0], ""
				if len(args) > 1 {
					revB = args[1]
				} else if parts := strings.SplitN(revA, "..", 2); len(parts) == 2 {
					revA, revB = parts[0], strings.TrimPrefix(parts[1], ".")
				}
				patch, err := outgit.DiffCommits("", revA, revB)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), patch)
				return nil
			}
			_, entries, err := outgit.Status(".")
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			label := "Working tree changes:"
			wantStaged := staged
			if wantStaged {
				label = "Staged changes:"
			}
			matched := false
			for _, e := range entries {
				if e.Staged == wantStaged {
					if !matched {
						fmt.Fprintln(out, label)
						matched = true
					}
					fmt.Fprintf(out, "  %s  %s\n", e.Status, e.File)
				}
			}
			if !matched {
				fmt.Fprintln(out, "no changes")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&staged, "staged", false, "Show staged (index) changes instead of working-tree changes")
	return cmd
}

func gitRemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remote",
		Short:   "List configured remotes",
		Example: `  bashy git remote`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, remotes, err := outgit.Remotes(".")
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, r := range remotes {
				for _, u := range r.URLs {
					fmt.Fprintf(out, "%s\t%s\n", r.Name, u)
				}
			}
			return nil
		},
	}
}

func gitShowCmd() *cobra.Command {
	var noPatch bool
	cmd := &cobra.Command{
		Use:   "show [commit]",
		Short: "Show a commit: metadata, message, and the patch it introduced",
		Args:  cobra.MaximumNArgs(1),
		Example: `  bashy git show
  bashy git show HEAD~1
  bashy git show --no-patch v1.2.3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			commit := ""
			if len(args) > 0 {
				commit = args[0]
			}
			_, info, err := outgit.Show(outgit.ShowOptions{Commit: commit})
			if err != nil {
				// Host git exits 128 on every show runtime failure
				// (bad object, no repo); carry it through the door.
				return &gitExitError{code: 128, msg: err.Error()}
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "commit %s\n", info.Hash)
			fmt.Fprintf(out, "Author: %s <%s>\n", info.Author, info.Email)
			fmt.Fprintf(out, "Date:   %s\n\n", info.Date)
			fmt.Fprintln(out, info.Message)
			if !noPatch && info.Patch != "" {
				fmt.Fprint(out, info.Patch)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&noPatch, "no-patch", "s", false, "Suppress the patch output (metadata + message only)")
	return cmd
}

// gitRevParseCmd implements the subset of `git rev-parse` bashy's
// build scripts need to stamp the binary with commit + dirty metadata
// without shelling out to system git. Three forms:
//
//	bashy git rev-parse HEAD                  → full 40-char SHA
//	bashy git rev-parse --short[=N] HEAD      → abbreviated SHA (default 7)
//	bashy git rev-parse --is-dirty            → "true"/"false" + exit 0/1
//
// --is-dirty is a bashy convenience, not real git: it returns the
// same boolean the Makefile derived from `git diff --quiet` so the new
// scripts can compute the ldDirty stamp pure-Go.
func gitRevParseCmd() *cobra.Command {
	var (
		short    int
		shortSet bool
		isDirty  bool
	)
	cmd := &cobra.Command{
		Use:   "rev-parse [HEAD]",
		Short: "Resolve HEAD to a SHA (or report worktree dirtiness)",
		Args:  cobra.MaximumNArgs(1),
		Example: `  bashy git rev-parse HEAD
  bashy git rev-parse --short HEAD
  bashy git rev-parse --short=12 HEAD
  bashy git rev-parse --is-dirty`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Only HEAD is supported as the rev argument — that's
			// the only thing the build scripts ever ask about.
			if len(args) == 1 && args[0] != "HEAD" {
				return fmt.Errorf("rev-parse: only HEAD is supported, got %q", args[0])
			}
			opts := outgit.RevParseOptions{}
			if shortSet {
				if short <= 0 {
					short = 7
				}
				opts.Short = short
			}
			res, err := outgit.RevParse(opts)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if isDirty {
				// Match `git diff --quiet` exit-code semantics:
				// 0 = clean, 1 = dirty. Stdout always reports
				// "true"/"false" so scripts can read either signal.
				if res.Dirty {
					fmt.Fprintln(out, "true")
					dispatchExit(1)
				}
				fmt.Fprintln(out, "false")
				return nil
			}
			if opts.Short > 0 {
				fmt.Fprintln(out, res.Short)
				return nil
			}
			fmt.Fprintln(out, res.Hash)
			return nil
		},
	}
	cmd.Flags().IntVar(&short, "short", 7, "Abbreviate SHA to N chars when --short is passed; full 40-char SHA when omitted (bare --short uses N=7)")
	// NoOptDefVal lets `bashy git rev-parse --short HEAD` work — without
	// it, cobra greedily consumes "HEAD" as the value for --short and
	// fails to parse it as int. Passing --short alone now means "use 7".
	cmd.Flags().Lookup("short").NoOptDefVal = "7"
	cmd.Flags().BoolVar(&isDirty, "is-dirty", false, "Print true/false and exit 0/1 based on worktree dirtiness")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		shortSet = cmd.Flags().Changed("short")
		return nil
	}
	return cmd
}

// legacyGitLogArgs reports whether args fit the dedicated log command's own
// shape: -n/--number N plus at most one repository path.
func legacyGitLogArgs(args []string) bool {
	pos := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" || a == "--number":
			i++
			if i >= len(args) {
				return false
			}
		case strings.HasPrefix(a, "--number="):
		case strings.HasPrefix(a, "-"):
			return false
		default:
			pos++
		}
	}
	return pos <= 1
}
