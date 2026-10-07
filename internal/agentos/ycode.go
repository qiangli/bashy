package agentos

// YcodeMain runs the ycode CLI (github.com/qiangli/ycode/pkg/ycodecli.Main)
// with its arguments and returns the exit status. cmd/bashy sets it: ycode
// imports bashy's pkg/harnessrunner, which imports this package, so this
// package cannot import ycode itself. Nil in a build that does not link ycode.
var YcodeMain func(args []string) int

// YcodeHasExplicitConfig is wired beside YcodeMain by the embedding binary.
// It uses ycode's authored bootstrap parser to preserve flag value ownership.
var YcodeHasExplicitConfig func(args []string) bool
