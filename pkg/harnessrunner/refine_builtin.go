package harnessrunner

import "github.com/qiangli/coreutils/pkg/atlas"

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
