# Default kb pages

Generic, user-facing knowledge that every Bashy install can read without
anything compiled into the binary. Installs fetch this tree at the release
pin (`bashy src fetch bashy`), and `bashy kb search|list|show` and
`bashy recall`/`context` read `${BASHY_HOME:-$HOME/.bashy}/src/bashy/kb/defaults`
as a read-only ring after the writable ones; a local page with the same slug
always wins. `$BASHY_KB_DEFAULTS_DIR` points elsewhere.

Curation rules:

- Only knowledge useful to any Bashy user (setup, how-to, gotchas); repo
  development notes stay in `docs/kb`.
- Generic names only: variables for host, user and home (`$HOME`,
  `$(hostname)`, `${XDG_CONFIG_HOME:-$HOME/.config}`), placeholders for
  accounts, teams and secret names. No hosts, users, IPs, endpoints or
  credentials.
- Copy a page from `docs/kb/pages` once it is verified, then run
  `bashy kb --dir kb/defaults index`.
