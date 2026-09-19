package agentos

import (
	"context"

	"github.com/qiangli/yoke/pkg/weave"
)

// The whole test binary runs with NO relay: the production seam derives a
// session from the cwd's origin and the paired outpost token, which in this
// package's tests is the operator's real checkout and real cloudbox. A test
// that wants a relay installs its own fake and restores this one.
func init() {
	deliverSessionMail = func(context.Context, string, string) (weave.DeliveryReport, error) {
		return weave.DeliveryReport{Skipped: "test binary"}, nil
	}
}
