---
id: f39a897d8cd4
kind: feature
title: S601.3 Gmail Threads, Reply & Bounded Attachment helper (examples/google/gmail_threads.py)
seq: 3201
status: todo
priority: p1
created: 2026-11-09T09:04:53.95651Z
sprint: 601
sprint_id: 3f93eee1-fb25-5828-a779-966c345ff576
sprint_title: Google Workspace custom command MVP (examples/google)
---

Create standalone helper module examples/google/gmail_threads.py and hermetic unit test suite examples/google/test_gmail_threads.py (do not modify google.bsh or other files). Implement CLI/callable functions wrapping gws (via GWS_BIN or gws on PATH with GWS_ACCOUNT and GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE preflight and SECRET_VIA_ARGV refusal):
1. gmail-thread-get <thread-id> [format=full]: fetches a Gmail thread via 'gws gmail users threads get --params {"userId":"me","id":...,"format":...} --format=json' and summarizes messages (id, from, subject, date, snippet).
2. gmail-reply <message-id> body=TEXT [cc=ADDR] [--dry-run]: fetches the original message headers (Subject, From, Reply-To, Message-ID, References, threadId), constructs an RFC 2822 reply with 'Re: ' subject, In-Reply-To, and References headers, base64url-encodes it, and sends via 'gws gmail users messages send --params {"userId":"me"} --json {"raw":...,"threadId":...} --format=json'.
3. gmail-attachment-get <message-id> <attachment-id> [output=<path>]: fetches attachment via 'gws gmail users messages attachments get --params {"userId":"me","messageId":...,"id":...} --format=json', enforces 25 MiB size cap (MAX_ATTACHMENT_BYTES = 26214400), decodes base64url data, and writes to output path or returns metadata.
Verify with: python3 -m unittest examples/google/test_gmail_threads.py -v
