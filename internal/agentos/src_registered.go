package agentos

import (
	"os"
	"path/filepath"

	"github.com/qiangli/yoke/pkg/assetring"
	"github.com/qiangli/yoke/pkg/src"
)

func init() { src.RegisteredSource = registeredSourceFor }

// registeredSourceFor answers `bashy src show NAME` for a command registered
// with `bashy commands add`: the file that implements it, so an agent can use
// it as is or change and rebuild it like any skill-tier unit. For an exec
// entry that is the script it runs (e.g. `bashy --bashsharp …/record.bsh`) or
// the program itself; for an inline script or a download it is the registry
// record that holds it.
func registeredSourceFor(name string) (string, string, bool) {
	rec, ok := registeredLookup(name)
	if !ok {
		return "", "", false
	}
	if len(rec.Exec) > 0 {
		for _, a := range rec.Exec[1:] {
			if isSourceFile(a) {
				return src.RegisteredRepo, a, true
			}
		}
		if filepath.IsAbs(rec.Exec[0]) && isSourceFile(rec.Exec[0]) {
			return src.RegisteredRepo, rec.Exec[0], true
		}
	}
	if rec.Ring == assetring.RingLocal {
		if p, err := registeredCatalog().MaterializeCommand(rec.Name); err == nil {
			return src.RegisteredRepo, p, true
		}
	}
	return src.RegisteredRepo, "registry:" + rec.Name, true
}

func isSourceFile(p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
