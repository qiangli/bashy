package main

// TargetType classifies the kind of artifact to scan.
type TargetType int

const (
	TargetGoModule TargetType = iota
	TargetBinary
)

func (t TargetType) String() string {
	switch t {
	case TargetGoModule:
		return "module"
	case TargetBinary:
		return "binary"
	default:
		return "unknown"
	}
}

// Target represents an artifact path and its classified type.
type Target struct {
	Path string     `json:"path"`
	Type TargetType `json:"type"`
}
