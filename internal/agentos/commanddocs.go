// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"github.com/qiangli/bashy/skills"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

// commandDocSource bridges a front-door command's moved doc
// (skills/bashy/commands/<name>.md, its optional skill.dhnt and
// skills/bashy/reference/<name>.md companions) back into the skill catalog
// under its own name — so `bashy skill show/run <name>` keeps resolving a
// command's full skill body after its top-level skill folder folded into
// bashy's (Sprint 406). It is a Source like any other ring; the bridge is
// generic over every name skills.CommandNames reports, not one per command.
type commandDocSource struct{}

func (commandDocSource) Ring() coreskills.Ring { return coreskills.RingEmbedded }

func (commandDocSource) Names() ([]string, error) {
	return skills.CommandNames(), nil
}

func (commandDocSource) Body(name string) ([]byte, bool) {
	doc, ok := skills.CommandDoc(name)
	if !ok {
		return nil, false
	}
	return []byte(doc), true
}

func (commandDocSource) Files(name string) ([]string, error) {
	var out []string
	if _, ok := skills.CommandDoc(name); ok {
		out = append(out, "SKILL.md")
	}
	if _, ok := skills.CommandReference(name); ok {
		out = append(out, "reference.md")
	}
	if _, ok := skills.CommandDhnt(name); ok {
		out = append(out, "skill.dhnt")
	}
	return out, nil
}

func (commandDocSource) File(name, rel string) ([]byte, bool) {
	var (
		doc string
		ok  bool
	)
	switch rel {
	case "SKILL.md":
		doc, ok = skills.CommandDoc(name)
	case "reference.md":
		doc, ok = skills.CommandReference(name)
	case "skill.dhnt":
		doc, ok = skills.CommandDhnt(name)
	}
	if !ok {
		return nil, false
	}
	return []byte(doc), true
}
