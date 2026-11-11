---
name: suite
description: Manage Cloudbox app suites and share them with members or circles.
---

# suite — app suites and circle grants

`bashy suite` manages named collections of hosted apps with the Cloudbox
account owner credential. Each app is identified as `HOST/APP`. `set` creates
or replaces a suite's app list; `add` and `remove` change one app at a time.

```sh
bashy suite ls --json
bashy suite show work --json
bashy suite set work host1/calendar host1/notes --dry-run --json
bashy suite set work host1/calendar host1/notes --json
bashy suite add work host2/viewer --dry-run --json
bashy suite remove work host2/viewer --dry-run --json
bashy suite share work --circle team --dry-run --json
bashy suite share work --circle team --json
bashy suite share work --email MEMBER_EMAIL --dry-run --json
bashy suite shares work --json
bashy suite unshare work SHARE_ID --dry-run --json
bashy suite rm work --dry-run --json
```

`share` accepts exactly one of `--circle` or `--email`. A circle is an
owner-chosen lowercase slug of up to 32 characters; `family`, `friend`,
`team`, and `guest` are defaults, while `public` and `owner` are reserved.
Circle access is live: membership changes affect access to the suite without
recreating its share grant. Use `shares` to get the grant ID for `unshare`.
Use `--dry-run` to inspect a write before sending it. `--json` is available
on every subcommand.
