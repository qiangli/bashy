package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qiangli/bashy/optional/internal/toolrun"
	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/pflag"
)

var cmd = &tool.Tool{
	Name:     "dir",
	Synopsis: "List directory contents (accepts the same options as ls).",
	Usage:    "dir [OPTION]... [FILE]...",
}

func init() {
	cmd.Run = run
	tool.Register(cmd)
}

func main() {
	toolrun.Main("dir")
}

const argTakingShorts = "IwT"

func getFlagSet(name string) *pflag.FlagSet {
	fs := tool.NewFlags(name)
	fs.BoolP("all", "a", false, "do not ignore entries starting with .")
	fs.BoolP("almost-all", "A", false, "do not list implied . and ..")
	fs.BoolP("directory", "d", false, "list directories themselves, not their contents")
	fs.BoolP("human-readable", "h", false, "with -l, print sizes like 1K 234M 2G etc.")
	fs.BoolP("inode", "i", false, "print the index number of each file")
	fs.BoolP("recursive", "R", false, "list subdirectories recursively")
	fs.BoolP("reverse", "r", false, "reverse order while sorting")
	fs.BoolP("long", "l", false, "display detailed information")
	fs.BoolP("no-group", "G", false, "in a long listing, don't print group names")
	fs.BoolP("numeric-uid-gid", "n", false, "like -l, but list numeric user and group IDs")
	fs.String("format", "", "set display format: long, single-column, commas")
	fs.String("sort", "", "sort by WORD: name, none, time, size, extension")
	fs.StringArray("hide", nil, "do not list implied entries matching shell PATTERN")
	fs.StringArrayP("ignore", "I", nil, "do not list implied entries matching shell PATTERN")
	fs.BoolP("ignore-backups", "B", false, "do not list implied entries ending with ~")
	fs.Bool("zero", false, "end each output line with NUL, not newline")
	fs.Bool("file-type", false, "append file type indicators except '*'")
	fs.StringP("classify", "F", "", "append file type indicators")
	fs.String("indicator-style", "", "append indicator with style WORD: none, slash, file-type, classify")
	fs.BoolP("literal", "N", false, "print entry names without quoting")
	fs.BoolP("quote-name", "Q", false, "enclose entry names in double quotes")
	fs.BoolP("escape", "b", false, "print C-style escapes for nongraphic characters")
	fs.Bool("hide-control-chars", false, "print question marks instead of nongraphic characters")
	fs.Bool("show-control-chars", false, "show nongraphic characters as-is")
	fs.String("quoting-style", "", "set quoting style: literal, c, escape")
	fs.Bool("group-directories-first", false, "group directories before files")
	fs.BoolP("dereference", "L", false, "show file information for symlink referents")
	fs.BoolP("dereference-command-line", "H", false, "follow command-line symlinks")
	fs.Bool("dereference-command-line-symlink-to-dir", false, "follow command-line symlinked directories")
	fs.Bool("author", false, "with -l, print the author of each file")
	fs.Bool("context", false, "print security context when available")
	fs.String("color", "never", "control color output; accepted for compatibility")
	fs.String("hyperlink", "never", "control hyperlink output; accepted for compatibility")
	fs.IntP("width", "w", 0, "set output width; accepted for compatibility")
	fs.IntP("tabsize", "T", 8, "set tab stops; accepted for compatibility")
	fs.String("time", "", "select timestamp field: mtime/modification, atime/access/use, ctime/status, birth/creation")
	fs.String("time-style", "", "set time style for -l; full-iso supported")
	fs.Bool("full-time", false, "like -l --time-style=full-iso")
	fs.String("block-size", "", "scale block counts; supports 1, K, KB")
	fs.Bool("si", false, "print human-readable sizes in powers of 1000")
	fs.BoolP("size", "s", false, "print allocated size of each file, in blocks")
	fs.BoolP("kibibytes", "k", false, "use 1024-byte blocks for allocated sizes")
	fs.BoolP("dired", "D", false, "produce Emacs dired offsets (not supported)")
	fs.BoolP("1", "1", false, "list one file per line")
	fs.BoolP("t", "t", false, "sort by modification time, newest first")
	fs.BoolP("S", "S", false, "sort by file size, largest first")
	fs.BoolP("v", "v", false, "natural sort of (version) numbers within text")
	fs.BoolP("g", "g", false, "like -l, but do not list owner")
	fs.BoolP("o", "o", false, "like -l, but do not list group information")
	fs.BoolP("C", "C", false, "list entries by columns")
	fs.BoolP("x", "x", false, "list entries by lines instead of by columns")
	fs.BoolP("p", "p", false, "append / indicator to directories")
	fs.BoolP("f", "f", false, "do not sort, enable -a")
	fs.BoolP("U", "U", false, "do not sort; list entries in directory order")
	fs.BoolP("X", "X", false, "sort alphabetically by entry extension")
	fs.BoolP("q", "q", false, "print question marks instead of nongraphic characters")
	fs.BoolP("c", "c", false, "with -lt: sort by, and show, ctime")
	fs.BoolP("u", "u", false, "with -lt: sort by, and show, atime")
	fs.BoolP("m", "m", false, "fill width with a comma separated list of entries")
	fs.BoolP("Z", "Z", false, "print security context when available")
	if verFlag := fs.Lookup("version"); verFlag != nil {
		verFlag.Shorthand = "V"
	}
	fs.Lookup("classify").NoOptDefVal = "always"
	fs.Lookup("color").NoOptDefVal = "always"
	fs.Lookup("hyperlink").NoOptDefVal = "always"
	return fs
}

func extractShort(args []string, chars string, fs *pflag.FlagSet) ([]string, map[byte]int) {
	found := map[byte]int{}
	seq := 0
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if strings.HasPrefix(a, "--") {
			rest = append(rest, a)
			name := a[2:]
			hasValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
				hasValue = true
			}
			if flag := fs.Lookup(name); !hasValue && flag != nil && flag.NoOptDefVal == "" && i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			kept := []byte{'-'}
			for j := 1; j < len(a); j++ {
				seq++
				if strings.IndexByte(chars, a[j]) >= 0 {
					found[a[j]] = seq
				} else {
					kept = append(kept, a[j])
				}
				if strings.IndexByte(argTakingShorts, a[j]) >= 0 {
					kept = append(kept, a[j+1:]...)
					break
				}
			}
			if len(kept) > 1 {
				rest = append(rest, string(kept))
			}
			continue
		}
		rest = append(rest, a)
	}
	return rest, found
}

func run(rc *tool.RunContext, args []string) int {
	fs := getFlagSet(cmd.Name)
	rest, short := extractShort(args, "ltS1gGnoCpfUXQNbqsvCxZHLV", fs)
	_, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if short['V'] > 0 {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	self := strings.TrimSpace(os.Getenv("BASHY_SELF"))
	if self == "" {
		self = "bashy"
	}
	lsArgs := make([]string, 0, len(args)+3)
	lsArgs = append(lsArgs, "ls", "-C", "-b")
	lsArgs = append(lsArgs, args...)
	c := exec.Command(self, lsArgs...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	c.Stdin = rc.In
	c.Stdout = rc.Out
	c.Stderr = rc.Err
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(rc.Err, "dir: %v\n", err)
		return 1
	}
	return 0
}
