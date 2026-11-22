// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"os"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/meet"
	// Registers the app-pair store used by external-sender meet dm.
	_ "github.com/qiangli/yoke/pkg/webconsole"
)

// wireMeet connects every host-owned meet seam in one place: permanent role
// and room-secretary activation (steward_meet.go) and the message-board seam
// (mb_seam.go). Keeping the assignments together makes the wiring testable.
func wireMeet() {
	meet.StartPermanentRole = startMeetPermanentRole
	meet.StartRoomSecretary = activateMeetRoomSecretary
	meet.ValidateRoomSecretary = validateMeetRoomSecretary
	meet.Notify = notifyMeetInvitation
	meet.FetchMB = fetchMeetMessageBoardPosts
	meet.PostMB = postMeetMessageBoardPost
}

func main() {
	// Multi-participant deliberation session: agentic CLIs + a human take
	// turns; a dedicated notes-only secretary keeps and files the minutes.
	wireMeet()
	os.Exit(adapter.RunCobra(meet.NewMeetCmd(), "bashy meet:", nil))
}
