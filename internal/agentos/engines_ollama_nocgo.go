//go:build bashy_engines && !cgo

package agentos

func init() { ollamaNeedsNewEngine = true }
