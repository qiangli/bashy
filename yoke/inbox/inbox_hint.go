package main

// inboxReaderHint is the one line every reader surface names, kept byte-identical
// with yoke pkg/bus's InboxReaderLine (copied, not imported, so the pinned-module
// build stays green before that sibling lands).
const inboxReaderHint = "Messages to you: bashy inbox (peek by default; --from/--search/--since; reply with bashy mb send)"
