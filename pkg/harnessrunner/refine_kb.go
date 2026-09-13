package harnessrunner

import (
	"os"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/scope"
)

type commandRefiner func(argv []string, allStatic bool, intent *Intent) bool

var commandRefiners = map[string]commandRefiner{
	"kb": refineKBCall,
}

var scopeResolveMu sync.Mutex

func refineAtlasCall(name string, argv []string, allStatic bool, intent *Intent) bool {
	refiner, ok := commandRefiners[name]
	if !ok {
		return false
	}
	return refiner(argv, allStatic, intent)
}

var kbReadOnlySubverbs = map[string]bool{
	"context":   true,
	"search":    true,
	"show":      true,
	"list":      true,
	"backlinks": true,
	"doctor":    true,
	"recall":    true,
	"log":       true,
	"index":     true,
	"sources":   true,
}

var kbWriteSubverbs = map[string]bool{
	"add":       true,
	"note":      true,
	"update":    true,
	"supersede": true,
	"validate":  true,
	"observe":   true,
	"transfer":  true,
	"retro":     true,
}

func refineKBCall(argv []string, allStatic bool, intent *Intent) bool {
	args := argv
	if len(args) > 1 && isBashyFrontDoor(args[0]) {
		args = args[1:]
	}
	if len(args) < 2 || args[0] != "kb" {
		return false
	}
	subverb := args[1]
	if kbReadOnlySubverbs[subverb] {
		appendKBEffects(intent, argv, []string{atlas.EffRead})
		return true
	}
	if kbWriteSubverbs[subverb] {
		if !allStatic {
			markUnsupported(intent, "dynamicCommand", strings.Join(argv, " "), "command name or arguments cannot be proven")
			return true
		}
		appendKBEffects(intent, argv, []string{atlas.EffRead, atlas.EffWrite})
		return true
	}
	return false
}

func appendKBEffects(intent *Intent, argv []string, kinds []string) {
	targets := kbEffectTargets(intent.Cwd, argv)
	for _, target := range targets {
		if target == "" {
			continue
		}
		for _, kind := range kinds {
			intent.Effects = append(intent.Effects, Effect{
				Kind:      kind,
				Scope:     atlas.TierWorkspace,
				Target:    target,
				Source:    "command:kb",
				Certainty: "exact",
			})
		}
	}
}

func kbEffectTargets(cwd string, argv []string) []string {
	switch kbExplicitRing(argv) {
	case string(scope.KindAgent):
		return []string{resolvedKBStore(cwd, scope.KindAgent)}
	case "host", string(scope.KindUser):
		return []string{resolvedKBStore(cwd, scope.KindUser)}
	case string(scope.KindRepo):
		return []string{resolvedKBStore(cwd, scope.KindRepo)}
	}
	targets := []string{resolvedKBStore(cwd, "")}
	if kbArgsNameAgentRing(argv) {
		if agent := resolvedKBStore(cwd, scope.KindAgent); agent != "" {
			targets = append(targets, agent)
		}
	}
	return targets
}

func resolvedKBStore(cwd string, forced scope.Kind) string {
	sc, err := resolveScopeAt(cwd, scope.Options{
		RepoSub: kb.RepoSub,
		HostDir: func() (string, error) {
			return kb.DefaultDir(), nil
		},
		AgentDir: func() (string, error) {
			return kb.AgentRingDir(), nil
		},
		ForceRepo:  forced == scope.KindRepo,
		ForceUser:  forced == scope.KindUser,
		ForceAgent: forced == scope.KindAgent,
	})
	if err != nil {
		return ""
	}
	return canonicalTarget(sc.Dir())
}

func resolveScopeAt(cwd string, opts scope.Options) (*scope.Scope, error) {
	scopeResolveMu.Lock()
	defer scopeResolveMu.Unlock()

	previous, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(cwd); err != nil {
		return nil, err
	}
	defer func() { _ = os.Chdir(previous) }()
	return scope.Resolve(opts)
}

func kbExplicitRing(argv []string) string {
	for i, arg := range argv {
		if arg == "--ring" && i+1 < len(argv) {
			return strings.TrimSpace(argv[i+1])
		}
		if strings.HasPrefix(arg, "--ring=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--ring="))
		}
	}
	return ""
}

func kbArgsNameAgentRing(argv []string) bool {
	for i, arg := range argv {
		if arg == "--ring" || arg == "--rings" {
			if i+1 < len(argv) && ringListNamesAgent(argv[i+1]) {
				return true
			}
			continue
		}
		if strings.HasPrefix(arg, "--ring=") && ringListNamesAgent(strings.TrimPrefix(arg, "--ring=")) {
			return true
		}
		if strings.HasPrefix(arg, "--rings=") && ringListNamesAgent(strings.TrimPrefix(arg, "--rings=")) {
			return true
		}
	}
	return false
}

func ringListNamesAgent(value string) bool {
	for _, ring := range strings.Split(value, ",") {
		if strings.TrimSpace(ring) == string(scope.KindAgent) {
			return true
		}
	}
	return false
}
