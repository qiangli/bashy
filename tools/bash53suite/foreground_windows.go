//go:build windows

package main

type fixtureTerminal struct{}

func openFixtureTerminal() (*fixtureTerminal, error) { return nil, nil }
func (*fixtureTerminal) handoff(int) error           { return nil }
func (*fixtureTerminal) restore() error              { return nil }
