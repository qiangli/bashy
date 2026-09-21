// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The registry, the inspect catalog and docs/decorators.md name the same
// predefined set — nothing predefined goes uncatalogued, nothing catalogued
// is fiction.
func TestDecoratorCatalogMatchesRegistryAndDocs(t *testing.T) {
	registry := map[string]bool{}
	for name := range nativeDecoratorSet(new(bytes.Buffer), func(fn nativeDecoratorFunc) nativeDecoratorFunc { return fn }) {
		registry[name] = true
	}
	catalog := map[string]bool{}
	for _, d := range decoratorCatalog {
		catalog[d.Name] = true
	}
	if !equalSets(registry, catalog) {
		t.Fatalf("registry %v != inspect catalog %v", keys(registry), keys(catalog))
	}
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "decorators.md"))
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `@([a-z]+)` \\|").FindAllStringSubmatch(string(doc), -1) {
		documented[m[1]] = true
	}
	delete(documented, "timed") // the engine's, not bashy's registry
	if !equalSets(registry, documented) {
		t.Fatalf("registry %v != docs/decorators.md %v", keys(registry), keys(documented))
	}
	minimum := 0
	for _, d := range decoratorCatalog {
		if d.Minimum {
			minimum++
		}
	}
	if minimum != 8 {
		t.Fatalf("the minimum set is 8 decorators, catalog says %d — growing it is a decision, not a drift", minimum)
	}
}

func equalSets(a, b map[string]bool) bool {
	return strings.Join(keys(a), ",") == strings.Join(keys(b), ",")
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
