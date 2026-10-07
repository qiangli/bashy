//go:build !bashy_core && !bashy_cert_base

package main

import (
	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/ycode/pkg/ycodecli"
)

func init() {
	agentos.YAMLAgentPrepare = func(source string, data []byte) (func() (agentos.YAMLAgentSession, error), error) {
		open, err := ycodecli.PrepareTextAgent(source, data)
		if err != nil {
			return nil, err
		}
		return func() (agentos.YAMLAgentSession, error) { return open() }, nil
	}
}
