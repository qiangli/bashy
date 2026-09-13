package harnessrunner

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/pkg/atlas"
	"mvdan.cc/sh/v3/syntax"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var builtinEffects = map[string][]string{
	":":      {atlas.EffPure},
	"echo":   {atlas.EffPure},
	"exit":   {atlas.EffPure},
	"false":  {atlas.EffPure},
	"printf": {atlas.EffPure},
	"pwd":    {atlas.EffRead},
	"read":   {atlas.EffRead},
	"true":   {atlas.EffPure},
}

var (
	toolNamesOnce sync.Once
	toolNames     map[string]bool
	execHashOnce  sync.Once
	execHash      string
	execHashErr   error
)

// CompileIntent produces a deterministic, fail-closed description of a
// command. Unsupported syntax is represented in the returned Intent rather
// than silently classified as safe.
func CompileIntent(req Request) (Intent, error) {
	return compileIntent(req, nil)
}

func compileIntent(req Request, secretKey []byte) (Intent, error) {
	if req.SchemaVersion != "" && req.SchemaVersion != RequestSchemaVersion {
		return Intent{}, fmt.Errorf("unsupported request schema %q", req.SchemaVersion)
	}
	if (req.Command.Script == "") == (len(req.Command.Argv) == 0) {
		return Intent{}, fmt.Errorf("exactly one of command.script and command.argv is required")
	}
	if req.Limits.WallTimeMs < 0 || req.Limits.StdoutBytes < 0 || req.Limits.StderrBytes < 0 || req.Limits.StdinBytes < 0 {
		return Intent{}, fmt.Errorf("limits cannot be negative")
	}
	cwd := req.Command.Cwd
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Intent{}, fmt.Errorf("resolve cwd: %w", err)
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Intent{}, fmt.Errorf("resolve cwd: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	} else {
		return Intent{}, fmt.Errorf("resolve cwd: %w", resolveErr)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return Intent{}, fmt.Errorf("cwd is not an accessible directory: %s", abs)
	}

	environment, err := compileEnvironment(req.Command.Environment, secretKey)
	if err != nil {
		return Intent{}, err
	}
	intent := Intent{
		Complete:    true,
		Cwd:         filepath.Clean(abs),
		Environment: environment,
		Limits:      req.Limits,
		Placement:   req.Placement,
	}
	for _, environmentFact := range environment {
		if environmentFact.Secret {
			intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffCred, Scope: atlas.TierUserland, Target: environmentFact.ValueRef, Source: "environment:" + environmentFact.Name, Certainty: "exact"})
		}
	}
	if req.Limits.StdinBytes > 0 {
		intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffWrite, Scope: "process", Target: "stdin", Source: "interactive-input", Certainty: "maximum"})
	}
	if req.Command.Script != "" {
		intent.ScriptDigest = digestBytes([]byte(req.Command.Script))
		if err := compileScript(req.Command.Script, &intent); err != nil {
			return Intent{}, err
		}
	} else {
		intent.Argv = append([]string(nil), req.Command.Argv...)
		compileCall(req.Command.Argv, true, &intent)
	}

	dedupeAndSortIntent(&intent)
	atlasView := struct {
		Commands []CommandFact `json:"commands"`
	}{Commands: intent.Commands}
	intent.AtlasDigest, err = digestJSON(atlasView)
	if err != nil {
		return Intent{}, err
	}
	canonical := intent
	canonical.Digest = ""
	intent.Digest, err = digestJSON(canonical)
	if err != nil {
		return Intent{}, err
	}
	return intent, nil
}

func compileEnvironment(values []EnvironmentVariable, secretKey []byte) ([]EnvironmentFact, error) {
	seen := make(map[string]bool, len(values))
	out := make([]EnvironmentFact, 0, len(values))
	for _, value := range values {
		if !environmentName.MatchString(value.Name) {
			return nil, fmt.Errorf("invalid environment name %q", value.Name)
		}
		if seen[value.Name] {
			return nil, fmt.Errorf("duplicate environment name %q", value.Name)
		}
		valueDigest := digestBytes([]byte(value.Value))
		if value.Secret {
			if len(secretKey) == 0 {
				return nil, fmt.Errorf("secret environment values require a keyed compiler")
			}
			mac := hmac.New(sha256.New, secretKey)
			_, _ = mac.Write([]byte("bashy.harness.environment/v1\x00" + value.Name + "\x00" + value.Value))
			valueDigest = "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
		}
		seen[value.Name] = true
		out = append(out, EnvironmentFact{
			Name: value.Name, ValueRef: value.ValueRef, Secret: value.Secret,
			ValueDigest: valueDigest,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func compileScript(script string, intent *Intent) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(script), "<harness>")
	if err != nil {
		return fmt.Errorf("parse command: %w", err)
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
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
			compileCall(argv, allStatic, intent)
		case *syntax.Redirect:
			compileRedirect(n, intent)
		}
		return true
	})
	return nil
}

func staticWord(word *syntax.Word) (string, bool) {
	var b strings.Builder
	for _, part := range word.Parts {
		switch value := part.(type) {
		case *syntax.Lit:
			b.WriteString(value.Value)
		case *syntax.SglQuoted:
			b.WriteString(value.Value)
		case *syntax.DblQuoted:
			for _, quotedPart := range value.Parts {
				literal, ok := quotedPart.(*syntax.Lit)
				if !ok {
					return "<dynamic>", false
				}
				b.WriteString(literal.Value)
			}
		default:
			return "<dynamic>", false
		}
	}
	return b.String(), true
}

func compileCall(argv []string, allStatic bool, intent *Intent) {
	if len(argv) == 0 {
		return
	}
	name, factArgv, commandStatic := commandName(argv)
	if !commandStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return
	}
	if strings.ContainsAny(name, `/\\`) {
		markUnsupported(intent, "executable", name, "path executables require a trusted digest-pinned resolver")
		return
	}
	if effects, ok := builtinEffects[name]; ok {
		if !allStatic && !pureBuiltin(name) {
			markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
			return
		}
		fact := CommandFact{Name: name, Argv: append([]string(nil), factArgv...), Resolver: "shell-builtin", Tier: atlas.TierUserland, Stage: atlas.StageCross, Shape: string(atlas.ShapeResult), MaximumEffects: append([]string(nil), effects...)}
		fact.ContentDigest, _ = executableDigest()
		if fact.ContentDigest == "" {
			markUnsupported(intent, "executableDigest", name, "running Bashy binary could not be digested")
		}
		intent.Commands = append(intent.Commands, fact)
		if name == "read" {
			intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffRead, Scope: "process", Target: "stdin", Source: "builtin:read", Certainty: "exact"})
		} else if name == "pwd" {
			intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffRead, Scope: atlas.TierWorkspace, Target: intent.Cwd, Source: "builtin:pwd", Certainty: "exact"})
		} else {
			appendMaximumEffects(intent, fact)
		}
		return
	}
	entry, ok := atlas.Lookup(name)
	if !ok || len(entry.Effects) == 0 {
		markUnsupported(intent, "command", name, "command is missing a complete Command Atlas classification")
		return
	}
	fact := CommandFact{
		Name: name, Argv: append([]string(nil), factArgv...), Resolver: "bashy-atlas",
		Group: entry.Group, Tier: entry.Tier, Stage: entry.Stage,
		Shape: string(entry.OutputShape()), Capabilities: append([]string(nil), entry.Caps...),
		MaximumEffects: append([]string(nil), entry.Effects...),
	}
	fact.ContentDigest, _ = executableDigest()
	if fact.ContentDigest == "" {
		markUnsupported(intent, "executableDigest", name, "running Bashy binary could not be digested")
	}
	intent.Commands = append(intent.Commands, fact)
	toolNamesOnce.Do(func() {
		toolNames = make(map[string]bool)
		for _, toolName := range atlas.ToolNames() {
			toolNames[toolName] = true
		}
	})
	_, hasCommandRefiner := commandRefiners[name]
	if !toolNames[name] && !hasCommandRefiner {
		markUnsupported(intent, "command", name, "front-door command requires a trusted digest-pinned executable resolver")
		return
	}
	if refineAtlasCall(name, factArgv, allStatic, intent) {
		return
	}
	if !allStatic {
		markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
		return
	}
	appendMaximumEffects(intent, fact)
	// Atlas supplies a conservative maximum, not argv semantics. Until a
	// command-specific refiner proves operands and destinations, only a pure
	// applet is effect-complete. Calling a broad maximum "complete" would let
	// `rm x` or `fetch URL` pass without binding x or URL.
	for _, effect := range entry.Effects {
		if effect != atlas.EffPure {
			markUnsupported(intent, "effectRefinement", name, "command arguments require a command-specific target/destination refiner")
			break
		}
	}
}

func commandName(argv []string) (string, []string, bool) {
	if argv[0] == "<dynamic>" {
		return "", argv, false
	}
	if len(argv) > 1 && isBashyFrontDoor(argv[0]) {
		if argv[1] == "<dynamic>" {
			return "", argv, false
		}
		return argv[1], argv, true
	}
	return argv[0], argv, true
}

func isBashyFrontDoor(name string) bool {
	switch filepath.Base(name) {
	case "bashy", "bashy.real":
		return true
	default:
		return false
	}
}

func appendMaximumEffects(intent *Intent, command CommandFact) {
	for _, effect := range command.MaximumEffects {
		intent.Effects = append(intent.Effects, Effect{Kind: effect, Scope: command.Tier, Source: "command:" + command.Name, Certainty: "maximum"})
	}
}

func compileRedirect(redir *syntax.Redirect, intent *Intent) {
	op := redir.Op.String()
	if strings.HasPrefix(op, "<<") || op == "<<<" {
		return
	}
	target, static := staticWord(redir.Word)
	if !static {
		markUnsupported(intent, "dynamicRedirection", op, "redirection target cannot be proven")
		return
	}
	if target == "" || target == "-" || strings.HasPrefix(target, "&") {
		return
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(intent.Cwd, target)
	}
	target = canonicalTarget(target)
	switch {
	case strings.Contains(op, ">"):
		intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffWrite, Scope: atlas.TierWorkspace, Target: target, Source: "redirection:" + op, Certainty: "exact"})
		if !strings.Contains(op, ">>") {
			intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffDestroy, Scope: atlas.TierWorkspace, Target: target, Source: "redirection:" + op, Certainty: "possible"})
		}
	case strings.Contains(op, "<"):
		intent.Effects = append(intent.Effects, Effect{Kind: atlas.EffRead, Scope: atlas.TierWorkspace, Target: target, Source: "redirection:" + op, Certainty: "exact"})
	default:
		markUnsupported(intent, "redirection", op, "redirection operator is not classified")
	}
}

func canonicalTarget(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	// For a not-yet-created leaf, resolve the nearest existing ancestor and
	// append the untouched suffix. This prevents a symlinked parent from
	// disguising the destination while preserving creation intent.
	parent := filepath.Dir(path)
	base := filepath.Base(path)
	if parent == path {
		return path
	}
	return filepath.Join(canonicalTarget(parent), base)
}

func markUnsupported(intent *Intent, kind, value, reason string) {
	intent.Complete = false
	intent.Unsupported = append(intent.Unsupported, UnsupportedFact{Kind: kind, Value: value, Reason: reason})
}

func dedupeAndSortIntent(intent *Intent) {
	effectSet := make(map[string]Effect, len(intent.Effects))
	for _, effect := range intent.Effects {
		key := effect.Kind + "\x00" + effect.Scope + "\x00" + effect.Target + "\x00" + effect.Source + "\x00" + effect.Certainty
		effectSet[key] = effect
	}
	hasGovernedEffect := false
	for _, effect := range effectSet {
		if effect.Kind != atlas.EffPure {
			hasGovernedEffect = true
			break
		}
	}
	if hasGovernedEffect {
		for key, effect := range effectSet {
			if effect.Kind == atlas.EffPure {
				delete(effectSet, key)
			}
		}
	}
	intent.Effects = intent.Effects[:0]
	keys := make([]string, 0, len(effectSet))
	for key := range effectSet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		intent.Effects = append(intent.Effects, effectSet[key])
	}
	sort.Slice(intent.Unsupported, func(i, j int) bool {
		a, b := intent.Unsupported[i], intent.Unsupported[j]
		return a.Kind+a.Value+a.Reason < b.Kind+b.Value+b.Reason
	})
	for i := range intent.Commands {
		sort.Strings(intent.Commands[i].Capabilities)
		sort.Strings(intent.Commands[i].MaximumEffects)
	}
}

func executableDigest() (string, error) {
	execHashOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			execHashErr = err
			return
		}
		file, err := os.Open(path)
		if err != nil {
			execHashErr = err
			return
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			execHashErr = err
			return
		}
		execHash = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	})
	return execHash, execHashErr
}
