package agentos

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	outgit "github.com/qiangli/coreutils/git"
)

// Parity verbs for `bashy git` beyond the original clone→push set:
// merge, merge-base, rev-list, config, tag, reset, rm, ls-files,
// blame, grep. Each is a thin cobra wrapper over the matching
// coreutils/git function — keep logic out of this file.

func gitMergeCmd() *cobra.Command {
	var (
		noFF    bool
		message string
	)
	cmd := &cobra.Command{
		Use:   "merge <branch|tag|commit>",
		Short: "Merge a ref into the current branch (fast-forward only)",
		Args:  cobra.ExactArgs(1),
		Example: `  bashy git merge feature-x
  bashy git merge --no-ff feature-x -m "Merge feature-x"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := outgit.Merge(outgit.MergeOptions{
				Ref:     args[0],
				NoFF:    noFF,
				Message: message,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noFF, "no-ff", false, "Record a merge commit even when fast-forward is possible")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Merge commit message (with --no-ff)")
	return cmd
}

func gitMergeBaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "merge-base <rev1> <rev2>",
		Short:   "Find the best common ancestor of two revisions",
		Args:    cobra.ExactArgs(2),
		Example: `  bashy git merge-base main feature-x`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bases, err := outgit.MergeBase("", args[0], args[1])
			if err != nil {
				return err
			}
			for _, b := range bases {
				fmt.Fprintln(cmd.OutOrStdout(), b)
			}
			return nil
		},
	}
}

func gitRevListCmd() *cobra.Command {
	var count bool
	cmd := &cobra.Command{
		Use:   "rev-list --count <rev | rev1..rev2>",
		Short: "Count commits in a range",
		Args:  cobra.ExactArgs(1),
		Example: `  bashy git rev-list --count HEAD
  bashy git rev-list --count main..feature-x`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !count {
				return fmt.Errorf("rev-list: only --count is supported")
			}
			n, err := outgit.RevListCount("", args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), n)
			return nil
		},
	}
	cmd.Flags().BoolVar(&count, "count", false, "Print the number of commits instead of listing them")
	return cmd
}

func gitConfigCmd() *cobra.Command {
	var (
		global bool
		unset  bool
	)
	cmd := &cobra.Command{
		Use:   "config <key> [value]",
		Short: "Get or set configuration (user.name, user.email, …)",
		Args:  cobra.RangeArgs(1, 2),
		Example: `  bashy git config user.name
  bashy git config user.name "Alice Example"
  bashy git config --global user.email alice@example.com
  bashy git config --unset user.name`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if len(args) == 1 && !unset {
				value, found, err := outgit.ConfigGet("", key)
				if err != nil {
					return err
				}
				if !found {
					// Match `git config <key>`: silent, exit 1.
					cmd.SilenceUsage = true
					cmd.SilenceErrors = true
					return fmt.Errorf("config: %s is not set", key)
				}
				fmt.Fprintln(cmd.OutOrStdout(), value)
				return nil
			}
			value := ""
			if len(args) == 2 {
				value = args[1]
			}
			result, err := outgit.ConfigSet("", key, value, global, unset)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Read/write the user-global config file instead of the repo's")
	cmd.Flags().BoolVar(&unset, "unset", false, "Remove the key")
	return cmd
}

func gitTagCmd() *cobra.Command {
	var (
		annotate bool
		message  string
		del      bool
		list     string
	)
	cmd := &cobra.Command{
		Use:   "tag [name] [commit]",
		Short: "List, create, or delete tags",
		Args:  cobra.MaximumNArgs(2),
		Example: `  bashy git tag                       # list
  bashy git tag -l 'v1.*'             # list matching
  bashy git tag v1.2.3                # lightweight tag at HEAD
  bashy git tag -a v1.2.3 -m "Release 1.2.3" abc1234
  bashy git tag -d v1.2.3             # delete`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if len(args) == 0 || cmd.Flags().Changed("list") {
				tags, err := outgit.TagList("", list)
				if err != nil {
					return err
				}
				for _, t := range tags {
					fmt.Fprintln(out, t)
				}
				return nil
			}
			if del {
				result, err := outgit.TagDelete("", args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(out, result.Message)
				return nil
			}
			commit := ""
			if len(args) > 1 {
				commit = args[1]
			}
			result, err := outgit.TagCreate(outgit.TagOptions{
				Name:     args[0],
				Commit:   commit,
				Message:  message,
				Annotate: annotate,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(out, result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&annotate, "annotate", "a", false, "Create an annotated tag")
	cmd.Flags().StringVarP(&message, "message", "m", "", "Tag message (implies annotated)")
	cmd.Flags().BoolVarP(&del, "delete", "d", false, "Delete the named tag")
	cmd.Flags().StringVarP(&list, "list", "l", "", "List tags matching a glob pattern")
	cmd.Flags().Lookup("list").NoOptDefVal = "*"
	return cmd
}

func gitResetCmd() *cobra.Command {
	var (
		soft  bool
		mixed bool
		hard  bool
	)
	cmd := &cobra.Command{
		Use:   "reset [--soft|--mixed|--hard] [commit]",
		Short: "Reset HEAD (and optionally index + working tree) to a commit",
		Long: `Reset the current branch to a commit. --soft moves HEAD only,
--mixed (default) also resets the index, --hard also resets the
working tree.

Caveat vs. real git: --hard additionally deletes untracked files
(a go-git behavior). Stash-equivalents don't exist here — move
anything precious out of the tree first.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  bashy git reset                  # unstage everything
  bashy git reset --hard HEAD~1    # drop the last commit`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := outgit.ResetMixed
			set := 0
			if soft {
				mode = outgit.ResetSoft
				set++
			}
			if mixed {
				mode = outgit.ResetMixed
				set++
			}
			if hard {
				mode = outgit.ResetHard
				set++
			}
			if set > 1 {
				return fmt.Errorf("reset: --soft, --mixed, and --hard are mutually exclusive")
			}
			commit := ""
			if len(args) > 0 {
				commit = args[0]
			}
			result, err := outgit.Reset(outgit.ResetOpts{Mode: mode, Commit: commit})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVar(&soft, "soft", false, "Move HEAD only")
	cmd.Flags().BoolVar(&mixed, "mixed", false, "Move HEAD and reset the index (default)")
	cmd.Flags().BoolVar(&hard, "hard", false, "Move HEAD, reset index and working tree (also removes untracked files)")
	return cmd
}

func gitRmCmd() *cobra.Command {
	var (
		cached    bool
		recursive bool
	)
	cmd := &cobra.Command{
		Use:   "rm <path>...",
		Short: "Remove files from the index and working tree",
		Args:  cobra.MinimumNArgs(1),
		Example: `  bashy git rm old.txt
  bashy git rm --cached secret.env    # untrack, keep on disk
  bashy git rm -r build/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := outgit.Rm(outgit.RmOptions{
				Paths:     args,
				Cached:    cached,
				Recursive: recursive,
			})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().BoolVar(&cached, "cached", false, "Remove from the index only; keep the working-tree file")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Allow recursive removal of directories")
	return cmd
}

func gitLsFilesCmd() *cobra.Command {
	var (
		modified bool
		others   bool
	)
	cmd := &cobra.Command{
		Use:   "ls-files",
		Short: "List tracked files (or modified/untracked with flags)",
		Example: `  bashy git ls-files
  bashy git ls-files -m     # modified
  bashy git ls-files -o     # untracked`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := outgit.LsFilesCached
			if modified && others {
				return fmt.Errorf("ls-files: -m and -o are mutually exclusive")
			}
			if modified {
				mode = outgit.LsFilesModified
			}
			if others {
				mode = outgit.LsFilesOthers
			}
			paths, err := outgit.LsFiles("", mode)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, p := range paths {
				fmt.Fprintln(out, p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&modified, "modified", "m", false, "List files with worktree modifications")
	cmd.Flags().BoolVarP(&others, "others", "o", false, "List untracked files")
	return cmd
}

func gitBlameCmd() *cobra.Command {
	var lineRange string
	cmd := &cobra.Command{
		Use:   "blame <file>",
		Short: "Show what commit and author last modified each line",
		Args:  cobra.ExactArgs(1),
		Example: `  bashy git blame main.go
  bashy git blame -L 10,20 main.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			start, end := 0, 0
			if lineRange != "" {
				parts := strings.SplitN(lineRange, ",", 2)
				if len(parts) != 2 {
					return fmt.Errorf("blame: -L wants start,end")
				}
				var err error
				if start, err = strconv.Atoi(parts[0]); err != nil {
					return fmt.Errorf("blame: bad -L start %q", parts[0])
				}
				if end, err = strconv.Atoi(parts[1]); err != nil {
					return fmt.Errorf("blame: bad -L end %q", parts[1])
				}
			}
			lines, err := outgit.Blame("", args[0], start, end)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, l := range lines {
				fmt.Fprintf(out, "%s (%s %s %4d) %s\n", l.Hash, l.Author, l.Date, l.LineNo, l.Text)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&lineRange, "lines", "L", "", "Annotate only lines start,end")
	return cmd
}

func gitGrepCmd() *cobra.Command {
	var (
		ignoreCase bool
		filesOnly  bool
	)
	cmd := &cobra.Command{
		Use:   "grep <pattern> [path...]",
		Short: "Search committed content (HEAD tree) with a regexp",
		Args:  cobra.MinimumNArgs(1),
		Example: `  bashy git grep "func main"
  bashy git grep -i todo internal/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			matches, err := outgit.Grep(outgit.GrepOptions{
				Pattern:    args[0],
				IgnoreCase: ignoreCase,
				FilesOnly:  filesOnly,
				Paths:      args[1:],
			})
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				// Match git grep: no output, exit 1.
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				return fmt.Errorf("no matches")
			}
			out := cmd.OutOrStdout()
			for _, m := range matches {
				if filesOnly {
					fmt.Fprintln(out, m.File)
					continue
				}
				fmt.Fprintf(out, "%s:%d:%s\n", m.File, m.LineNo, m.Content)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&ignoreCase, "ignore-case", "i", false, "Case-insensitive matching")
	cmd.Flags().BoolVarP(&filesOnly, "files-with-matches", "l", false, "Print only the names of matching files")
	return cmd
}
