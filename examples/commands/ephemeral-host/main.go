// ephemeral-host rents short-lived cloud machines for agents and makes sure
// they are given back: every host is created with a deadline and a budget
// cap, recorded in a ledger, and destroyed only if the ledger knows it.
//
// It is a sprint tool (Sprint 311), registered with bashy as a command
// (`bashy commands add ephemeral-host`), not a shipped bashy builtin.
//
// The ledger is not the security boundary. A provider token that can delete
// one machine can delete every machine its account (team) can see, so the
// token handed to this program must belong to an account that holds nothing
// but ephemeral hosts. The ledger stops cooperating agents from destroying a
// peer's box or overspending; the account boundary stops everything else.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := NewCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ephemeral-host:", err)
		os.Exit(1)
	}
}
