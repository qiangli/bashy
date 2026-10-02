// Package extensions stores optional language and toolchain registrations as
// data. It deliberately does not import a language runtime or execute a row.
package extensions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/polyglot"
)

const Schema = "bashy.extension/v1"

type Protocol struct {
	Major int `yaml:"major" json:"major"`
	Minor int `yaml:"minor" json:"minor"`
}

// Payload pins an external executable. Source is provenance, never shell text.
// No field here causes a download; verify reads the selected local file only.
type Payload struct {
	Version    string `yaml:"version" json:"version"`
	Source     string `yaml:"source" json:"source"`
	SHA256     string `yaml:"sha256" json:"sha256"`
	Executable string `yaml:"executable" json:"executable"`
}

type Record struct {
	Schema    string             `yaml:"schema" json:"schema"`
	Kind      string             `yaml:"kind" json:"kind"`
	Name      string             `yaml:"name" json:"name"`
	Aliases   []string           `yaml:"aliases,omitempty" json:"aliases,omitempty"`
	Stage     string             `yaml:"stage" json:"stage"`
	Protocol  Protocol           `yaml:"protocol" json:"protocol"`
	Effects   []string           `yaml:"effects" json:"effects"`
	Payloads  map[string]Payload `yaml:"payloads" json:"payloads"`
	Fences    []string           `yaml:"fences,omitempty" json:"fences,omitempty"`
	Toolchain string             `yaml:"toolchain,omitempty" json:"toolchain,omitempty"`
}

var (
	nameRE   = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	platRE   = regexp.MustCompile(`^(darwin|linux|windows)/(amd64|arm64|386|arm|riscv64|ppc64le|s390x)$`)
)

func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// BuiltinReserved protects the shipped fence and toolchain names. Optional
// data cannot replace a compiled language row or a pinned base provisioner.
func BuiltinReserved(kind, name string) bool {
	if kind == "language" {
		_, ok := polyglot.LookupLanguage(name)
		return ok
	}
	for _, base := range []string{"go", "cc", "c++", "cc-linker", "clang", "clang++", "python3", "node", "bun", "typescript", "rustc", "podman", "tofu", "kubectl", "helm", "skills", "cargo", "uv", "npm", "cmake", "make"} {
		if name == base {
			return true
		}
	}
	return false
}

func names(r Record) []string {
	out := append([]string{r.Name}, r.Aliases...)
	if r.Kind == "language" {
		out = append(out, r.Fences...)
	}
	return out
}

func (r *Record) normalize() {
	r.Name = normalize(r.Name)
	for i := range r.Aliases {
		r.Aliases[i] = normalize(r.Aliases[i])
	}
	for i := range r.Fences {
		r.Fences[i] = normalize(r.Fences[i])
	}
	r.Toolchain = normalize(r.Toolchain)
}

func (r Record) Validate(reserved func(kind, name string) bool) error {
	if r.Schema != Schema {
		return fmt.Errorf("schema %q: want %s", r.Schema, Schema)
	}
	if r.Kind != "language" && r.Kind != "toolchain" {
		return fmt.Errorf("kind %q: want language or toolchain", r.Kind)
	}
	if r.Stage != "optional" && r.Stage != "experimental" {
		return fmt.Errorf("stage %q: operator records must be optional or experimental", r.Stage)
	}
	if r.Protocol.Major != 1 || r.Protocol.Minor < 0 {
		return fmt.Errorf("unsupported runner protocol %d.%d", r.Protocol.Major, r.Protocol.Minor)
	}
	seen := map[string]bool{}
	for _, name := range append([]string{r.Name}, r.Aliases...) {
		if !nameRE.MatchString(name) {
			return fmt.Errorf("invalid %s name %q", r.Kind, name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate %s spelling %q", r.Kind, name)
		}
		seen[name] = true
		if reserved != nil && reserved(r.Kind, name) {
			return fmt.Errorf("%s spelling %q is reserved by Bashy", r.Kind, name)
		}
	}
	for _, fence := range r.Fences {
		if !nameRE.MatchString(fence) {
			return fmt.Errorf("invalid fence spelling %q", fence)
		}
		if reserved != nil && reserved("language", fence) {
			return fmt.Errorf("fence spelling %q is reserved by Bashy", fence)
		}
		// The canonical language name may also be its fence tag.
		if seen[fence] && fence != r.Name {
			return fmt.Errorf("duplicate fence spelling %q", fence)
		}
		seen[fence] = true
	}
	if r.Kind == "toolchain" && (len(r.Fences) != 0 || r.Toolchain != "") {
		return fmt.Errorf("toolchain cannot declare fences or another toolchain")
	}
	if r.Kind == "language" && len(r.Fences) == 0 {
		return fmt.Errorf("language needs at least one fence spelling")
	}
	if len(r.Effects) == 0 {
		return fmt.Errorf("effects must be declared")
	}
	allowedEffects := map[string]bool{}
	for _, e := range atlas.Effects() {
		allowedEffects[e] = true
	}
	for _, e := range r.Effects {
		if !allowedEffects[e] {
			return fmt.Errorf("unknown effect %q", e)
		}
	}
	if len(r.Payloads) == 0 {
		return fmt.Errorf("at least one platform payload is required")
	}
	for platform, p := range r.Payloads {
		if !platRE.MatchString(platform) {
			return fmt.Errorf("unsupported platform %q", platform)
		}
		if p.Version == "" || p.Version == "latest" || p.Source == "" {
			return fmt.Errorf("%s needs an immutable version and source", platform)
		}
		if !sha256RE.MatchString(p.SHA256) {
			return fmt.Errorf("%s needs a lowercase SHA-256 digest", platform)
		}
		if !filepath.IsAbs(p.Executable) {
			return fmt.Errorf("%s executable must be an absolute path", platform)
		}
	}
	return nil
}

type Store struct {
	Root     string
	Reserved func(kind, name string) bool
}

func NewStore(reserved func(kind, name string) bool) Store {
	return Store{Root: fleet.DefaultRoot(), Reserved: reserved}
}

func (s Store) dir(kind string) (string, error) {
	switch kind {
	case "language":
		return filepath.Join(s.Root, "languages"), nil
	case "toolchain":
		return filepath.Join(s.Root, "toolchains"), nil
	}
	return "", fmt.Errorf("unknown extension kind %q", kind)
}

func (s Store) path(kind, name string) (string, error) {
	dir, err := s.dir(kind)
	if err != nil {
		return "", err
	}
	if !nameRE.MatchString(name) {
		return "", fmt.Errorf("invalid %s name %q", kind, name)
	}
	return filepath.Join(dir, name+".yaml"), nil
}

func (s Store) List(kind string) ([]Record, error) {
	dir, err := s.dir(kind)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		p, err := s.path(kind, name)
		if err != nil {
			return nil, err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var r Record
		dec := yaml.NewDecoder(strings.NewReader(string(body)))
		dec.KnownFields(true)
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if err := dec.Decode(&struct{}{}); err != io.EOF {
			return nil, fmt.Errorf("%s: multiple documents or trailing data", p)
		}
		r.normalize()
		if r.Name != name || r.Kind != kind {
			return nil, fmt.Errorf("%s: record identity differs from its path", p)
		}
		if err := r.Validate(s.Reserved); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s Store) Show(kind, name string) (Record, error) {
	rows, err := s.List(kind)
	if err != nil {
		return Record{}, err
	}
	name = normalize(name)
	for _, r := range rows {
		for _, n := range names(r) {
			if n == name {
				return r, nil
			}
		}
	}
	return Record{}, fmt.Errorf("%s %q is not registered", kind, name)
}

func (s Store) Save(r Record, replacing bool) error {
	r.normalize()
	if err := r.Validate(s.Reserved); err != nil {
		return err
	}
	rows, err := s.List(r.Kind)
	if err != nil {
		return err
	}
	path, err := s.path(r.Kind, r.Name)
	if err != nil {
		return err
	}
	_, statErr := os.Lstat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	if !replacing && statErr == nil {
		return fmt.Errorf("%s %q already exists", r.Kind, r.Name)
	}
	if replacing && os.IsNotExist(statErr) {
		return fmt.Errorf("%s %q is not registered", r.Kind, r.Name)
	}
	for _, existing := range rows {
		if replacing && existing.Name == r.Name {
			continue
		}
		for _, a := range names(r) {
			for _, b := range names(existing) {
				if a == b {
					return fmt.Errorf("%s spelling %q belongs to %q", r.Kind, a, existing.Name)
				}
			}
		}
	}
	body, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+r.Name+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (s Store) Remove(kind, name string) error {
	r, err := s.Show(kind, name)
	if err != nil {
		return err
	}
	p, err := s.path(kind, r.Name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Verify performs no network access or execution. It checks the selected
// platform's on-disk executable against the record's exact digest.
func (s Store) Verify(kind, name string) error {
	r, err := s.Show(kind, name)
	if err != nil {
		return err
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	p, ok := r.Payloads[platform]
	if !ok {
		return fmt.Errorf("%s %q has no payload for %s", kind, r.Name, platform)
	}
	f, err := os.Open(p.Executable)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("payload %q is not a regular file", p.Executable)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return fmt.Errorf("payload %q is not executable", p.Executable)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != p.SHA256 {
		return fmt.Errorf("%s %q: payload SHA-256 mismatch: got %s", kind, r.Name, got)
	}
	return nil
}
