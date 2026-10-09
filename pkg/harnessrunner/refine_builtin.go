package harnessrunner

import "github.com/qiangli/yoke/pkg/atlas"

func pureBuiltin(name string) bool {
	effects, ok := builtinEffects[name]
	if !ok {
		return false
	}
	for _, effect := range effects {
		if effect != atlas.EffPure {
			return false
		}
	}
	return true
}

func readOnlyTestBuiltin(name string) bool {
	switch name {
	case "test", "[", "[[":
		return true
	default:
		return false
	}
}

func pureOrReadOnlyBuiltin(name string) bool {
	return pureBuiltin(name) || readOnlyTestBuiltin(name)
}
