---
name: member
description: Manage Cloudbox account members and check mutual membership from the shell.
---

# member — account membership

`bashy member` reads and changes the Cloudbox account member list using the
paired account credential. An outpost pairs once with the Cloudbox portal for
its identity; satellites attach through mutual account membership and copy the
master list. Pairing does not create a tunnel.

```sh
bashy member ls --json
bashy member ls --circle family --json
bashy member invite EMAIL --circle friend --dry-run --json
bashy member invite EMAIL --circle friend --json
bashy member add EMAIL --circle friend --json
bashy member invitations --json
bashy member accept INVITATION_ID --json
bashy member of --json
bashy member set EMAIL --circle family --dry-run --json
bashy member set EMAIL --name "Display Name" --json
bashy member rm EMAIL --dry-run --json
bashy member check EMAIL --json
```

A circle is any owner-chosen name: a lowercase slug of up to 32 characters;
`public` and `owner` are reserved. `family`, `friend`, `team`, and `guest`
are the defaults, and a new member defaults to `guest`. A member is in one
circle at a time; share a suite with several circles to combine audiences.
`invite` emails the invitation; `add` records the same pending invitation
without email, and the member accepts it after signing in. `set --name`
relabels a member in your list. A check allows only
an active, mutual member from a fresh list. `ls` can show a cached list while
offline and marks it stale; that list cannot authorize a write or access.
Use `--dry-run` to inspect a write request before sending it. `--json` is
available on every subcommand.
