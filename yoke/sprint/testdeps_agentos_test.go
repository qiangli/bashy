package main

import (
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
)

// fleetNoun folds a fleet noun's hidden plural spelling onto its canonical
// singular. whois has no number.
func fleetNoun(n string) string {
	switch n {
	case "tools":
		return "tool"
	case "models":
		return "model"
	case "agents":
		return "agent"
	case "people":
		return "person"
	}
	return n
}
