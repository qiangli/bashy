---
id: 64113e7565ab
kind: test
title: Run native macOS Bash 5.3 86-fixture harness on final Sprint 253 pins
seq: 317
status: done
priority: p1
labels:
    - macos
created: 2026-09-23T06:41:14.757752Z
assignee: macos-s253-codex
sprint: 256
sprint_id: e8e29353-83f0-5b96-b555-c6e6e88bb09e
sprint_title: Verify final Sprint 253 Bash 5.3 candidate on macOS
closed: 2026-09-23T06:49:55.419801Z
closed_by: macos-s253-codex
---

Native macOS 26.3 arm64 verification of final Sprint 253 code with Go 1.27.1.
Source archived into an isolated tree: bashy b192f5f, sh f9bfd142, coreutils 84369e33, yoke 0868b481; auxiliary siblings bashsharp 57d18dc, readline 38ad08e, filebrowser c350ca3e. The pinned Bash 5.3 fixture tree was copied into the isolated tree. The serial `make test-bash` run used a custom `BASH_TESTS_DIR` inside that tree and a PTY so fixtures had a controlling terminal. The full log is retained in the isolated run directory as `mac-test-bash-tty.log`. Final Results: 86 passed, 0 failed, 0 skipped, 0 timed out; 86 PASS lines and no FAIL/TIME/SKIP lines. Exit 0. Testee SHA-256 bb099daae67344e2f0b2e6a592f76c2ab0d1a4f0c36fa1284455ce0bc0833d1b. Harness SHA-256 8770d5ae97dc692c4ed71e51c331c9d0361fab9f4d4ba5c304b20d6ada9b6023.
Initial non-TTY run: 82 passed, 4 failed (jobs/read/test/vredir), 0 skipped/timed out; each failure involved missing /dev/tty. It was rerun with a PTY and all four passed. The initial log is retained as `mac-test-bash.log` in the same isolated run directory.
