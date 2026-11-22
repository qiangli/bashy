# update

`bashy update` builds bashy from its main branch in its own build checkout
(`~/.bashy/src/bashy-update` by default), backs up the bashy currently on PATH
(`bashy.bak-<timestamp>` beside it), installs the new pair with the atomic
installer (`tools/installbashy`), and prints the old and new versions.

It is for humans. Agent sessions are not allowed to replace the shell binary
they run in, so `update` refuses inside one. `BASHY_UPDATE_ALLOW_AGENT=1` is the
explicit override, logged to `~/.bashy/log/update.log`.

    bashy update               # build origin/main and install
    bashy update --dry-run     # show the plan and the current version
    bashy update --ref v1.0.0-rc.40-dev

Register it once from a stable checkout (not a temporary copy, which a later
cleanup could delete):

    bash examples/update/register.sh

After updating, restart agent sessions, and `outpost restart` so services pick
up the new binary.
