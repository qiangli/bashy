package main

import (
	"os"
	"strings"

	_ "github.com/qiangli/yoke/pkg/fleet/fleetkinds"
	"github.com/qiangli/yoke/pkg/policy/coord"
)

func init() {
	if _, ok := coord.LookupKind("command"); !ok {
		coord.RegisterKind(coord.Kind{Name: "command", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("sandbox"); !ok {
		coord.RegisterKind(coord.Kind{Name: "sandbox", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("ollama"); !ok {
		coord.RegisterKind(coord.Kind{Name: "ollama", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("inbox"); !ok {
		coord.RegisterKind(coord.Kind{Name: "inbox", Match: coord.MatchName})
	}
}

// claimDisabled reports the explicit operator switch: BASHY_CLAIM=0|off.
func claimDisabled() bool {
	v := os.Getenv("BASHY_CLAIM")
	return v == "0" || strings.EqualFold(v, "off")
}
