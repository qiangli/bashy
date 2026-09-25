package harnessrunner

import (
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/atlas"
	"mvdan.cc/sh/v3/syntax"
)

// Functions and their Bash# decorators. A call to a function defined in the
// same script takes the effects of its body when the body is provable, the
// way a script's own commands do. Where the body cannot be proven command by
// command — an interpreter such as python or pytest — the function's Bash#
// declaration is the bound:
//
//	@effects("read,write,exec")   the envelope the author declares
//	@guard("read,write,exec")     a cap; @effects must lie within it
//	@contain(net: "deny")         children run with the network enforced off
//
// A declared envelope is recorded with scope "declared" and certainty
// "declared": the policy decides whether declared effects are acceptable.
// A declaration that leaves out `net` is only believed when the function is
// contained; otherwise the call keeps a possible net effect.

// ScopeDeclared labels effects that come from a Bash# declaration rather than
// from proven targets.
const ScopeDeclared = "declared"

type funcSummary struct {
	name      string
	body      *syntax.Stmt
	declared  []string // @effects, else @guard
	guard     []string
	contained bool
	decorated bool
	compiled  *Intent
	compiling bool
}

func collectFuncs(file *syntax.File) map[string]*funcSummary {
	funcs := map[string]*funcSummary{}
	syntax.Walk(file, func(node syntax.Node) bool {
		decl, ok := node.(*syntax.FuncDecl)
		if !ok || decl.Name == nil || decl.Body == nil {
			return true
		}
		summary := &funcSummary{name: decl.Name.Value, body: decl.Body}
		for _, d := range decl.Decorators {
			if d == nil || d.Name == nil {
				continue
			}
			value, argName := decoratorArg(d)
			switch d.Name.Value {
			case "effects":
				summary.declared = splitEffects(value)
				summary.decorated = true
			case "guard":
				summary.guard = splitEffects(value)
				summary.decorated = true
			case "contain":
				if (argName == "" || argName == "net") && value == "deny" {
					summary.contained = true
				}
			}
		}
		if summary.declared == nil {
			summary.declared = summary.guard
		}
		funcs[summary.name] = summary
		return true
	})
	return funcs
}

// decoratorArg returns the single static argument of a decorator and its
// keyword ("" when positional or not static).
func decoratorArg(d *syntax.BashPPDecorator) (string, string) {
	if len(d.Args) != 1 {
		return "", ""
	}
	value, ok := staticWord(d.Args[0])
	if !ok {
		return "", ""
	}
	name := ""
	if len(d.ArgNames) > 0 && d.ArgNames[0] != nil {
		name = d.ArgNames[0].Value
	}
	return value, name
}

func splitEffects(spec string) []string {
	var out []string
	for _, tok := range strings.Split(spec, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	sort.Strings(out)
	return out
}

func subset(inner, outer []string) bool {
	set := map[string]bool{}
	for _, e := range outer {
		set[e] = true
	}
	for _, e := range inner {
		if e != atlas.EffPure && !set[e] {
			return false
		}
	}
	return true
}

// compileFuncCall merges the effects of calling a script-defined function.
func compileFuncCall(summary *funcSummary, argv []string, intent *Intent, funcs map[string]*funcSummary) {
	if summary.compiling {
		markUnsupported(intent, "recursion", summary.name, "recursive function calls cannot be bounded")
		return
	}
	if summary.compiled == nil {
		summary.compiling = true
		body := &Intent{Complete: true, Cwd: intent.Cwd, cursor: intent.effectiveCwd()}
		walkCompile(summary.body, body, funcs)
		summary.compiling = false
		summary.compiled = body
	}
	body := summary.compiled
	intent.Commands = append(intent.Commands, body.Commands...)
	if body.cursor != "" {
		intent.cursor = body.cursor // a function runs in the calling shell: its cd persists
	}
	if summary.guard != nil && !subset(summary.declared, summary.guard) {
		markUnsupported(intent, "effectRefinement", summary.name, "declared effects exceed the function's guard")
		return
	}
	if body.Complete {
		for _, e := range body.Effects {
			if summary.decorated && !subset([]string{e.Kind}, summary.declared) {
				markUnsupported(intent, "effectRefinement", summary.name+": "+e.Kind, "the body's proven effects exceed its declaration")
				return
			}
		}
		intent.Effects = append(intent.Effects, body.Effects...)
		return
	}
	if !summary.decorated {
		for _, u := range body.Unsupported {
			markUnsupported(intent, u.Kind, u.Value, u.Reason)
		}
		return
	}
	// The body is not provable command by command: the declaration bounds it.
	target := "func:" + summary.name
	for _, kind := range summary.declared {
		if kind == atlas.EffPure {
			continue
		}
		intent.Effects = append(intent.Effects, Effect{Kind: kind, Scope: ScopeDeclared, Target: target, Source: "decorator:effects", Certainty: "declared"})
	}
	if !summary.contained && !subset([]string{atlas.EffNet}, summary.declared) {
		intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffNet, Scope: ScopeDeclared, Target: target, Source: "uncontained", Certainty: "possible"})
	}
	_ = argv
}

// walkCompile compiles every command under root into intent, resolving calls
// to script-defined functions through funcs. Function bodies are compiled at
// their call sites, never at their definitions.
func walkCompile(root syntax.Node, intent *Intent, funcs map[string]*funcSummary) {
	syntax.Walk(root, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.FuncDecl:
			return false
		case *syntax.CallExpr:
			if len(n.Args) == 0 {
				return true
			}
			argv := make([]string, 0, len(n.Args))
			allStatic := true
			for _, word := range n.Args {
				value, ok := staticWord(word)
				argv = append(argv, value)
				allStatic = allStatic && ok
			}
			if summary, ok := funcs[argv[0]]; ok {
				compileFuncCall(summary, argv, intent, funcs)
				return true
			}
			compileCall(argv, allStatic, intent)
		case *syntax.Redirect:
			compileRedirect(n, intent)
		}
		return true
	})
}
