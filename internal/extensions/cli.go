package extensions

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Run handles `bashy commands language|toolchain ...`. Registration remains
// data-only: these verbs never fetch or execute the declared payload.
func Run(store Store, kind string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: commands %s add|show|set|rm|verify NAME", kind)
	}
	verb := args[0]
	if verb == "view" {
		verb = "show"
	}
	if len(args) < 2 {
		return fmt.Errorf("%s needs a name", verb)
	}
	name := normalize(args[1])
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid %s name %q", kind, args[1])
	}
	sets := args[2:]
	switch verb {
	case "show":
		if len(sets) != 0 && !(len(sets) == 1 && sets[0] == "--json") {
			return fmt.Errorf("show accepts only --json")
		}
		r, err := store.Show(kind, name)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "verify":
		if len(sets) != 0 {
			return fmt.Errorf("verify takes no options")
		}
		if err := store.Verify(kind, name); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "%s %s: verified\n", kind, name)
		return err
	case "rm":
		if len(sets) != 0 {
			return fmt.Errorf("rm takes no options")
		}
		return store.Remove(kind, name)
	case "add", "set":
		var r Record
		if verb == "set" {
			var err error
			r, err = store.Show(kind, name)
			if err != nil {
				return err
			}
			if r.Name != name {
				return fmt.Errorf("set requires canonical name %q", r.Name)
			}
		} else {
			r = Record{Schema: Schema, Kind: kind, Name: name, Stage: "optional", Protocol: Protocol{Major: 1}, Payloads: map[string]Payload{}}
		}
		if len(sets) == 0 || len(sets)%2 != 0 {
			return fmt.Errorf("%s needs --set FIELD=VALUE pairs", verb)
		}
		for i := 0; i < len(sets); i += 2 {
			if sets[i] != "--set" {
				return fmt.Errorf("expected --set before %q", sets[i])
			}
			field, value, ok := strings.Cut(sets[i+1], "=")
			if !ok {
				return fmt.Errorf("--set needs FIELD=VALUE")
			}
			if err := apply(&r, field, value); err != nil {
				return err
			}
		}
		return store.Save(r, verb == "set")
	default:
		return fmt.Errorf("unknown %s verb %q", kind, verb)
	}
}

func csv(value string) []string {
	if value == "" {
		return nil
	}
	out := strings.Split(value, ",")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func apply(r *Record, field, value string) error {
	switch field {
	case "stage":
		r.Stage = value
	case "aliases":
		r.Aliases = csv(value)
	case "effects":
		r.Effects = csv(value)
	case "fences":
		r.Fences = csv(value)
	case "toolchain":
		r.Toolchain = value
	case "protocol.major":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("protocol.major: %w", err)
		}
		r.Protocol.Major = n
	case "protocol.minor":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("protocol.minor: %w", err)
		}
		r.Protocol.Minor = n
	default:
		if !strings.HasPrefix(field, "payloads.") {
			return fmt.Errorf("unknown extension field %q", field)
		}
		rest := strings.TrimPrefix(field, "payloads.")
		last := strings.LastIndexByte(rest, '.')
		if last < 0 {
			return fmt.Errorf("payload field needs platform and property")
		}
		platform, property := rest[:last], rest[last+1:]
		if !platRE.MatchString(platform) {
			return fmt.Errorf("unsupported platform %q", platform)
		}
		if r.Payloads == nil {
			r.Payloads = map[string]Payload{}
		}
		p := r.Payloads[platform]
		switch property {
		case "version":
			p.Version = value
		case "source":
			p.Source = value
		case "sha256":
			p.SHA256 = value
		case "executable":
			p.Executable = value
		default:
			return fmt.Errorf("unknown payload field %q", property)
		}
		r.Payloads[platform] = p
	}
	return nil
}
