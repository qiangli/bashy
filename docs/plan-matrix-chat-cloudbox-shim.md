# Matrix chat: Cloudbox account shim (S513.4a, Story #2740)

Bashy side of cloudbox commit `1b1af90` (`docs/matrix-chat.md` there).
Cloudbox maps each ordinary user to `@cb_<User.ID hex>:<server_name>`. It
gates native clients at its edge and calls a host shim to create accounts.

## Decisions

- **Account authority: a tuwunel application service, not an admin.**
  We probed tuwunel 1.9.4 (the signed bashy build) on loopback. An appservice
  declared in config, with an exclusive namespace `@cb_[0-9a-f]{32}:<server>`,
  can register namespace users with a password while `allow_registration =
  false`. It gets `M_EXCLUSIVE` outside the namespace. It is not a server
  admin. The AS and HS tokens come from vault-injected unit secrets
  (`TUWUNEL_APPSERVICE__CLOUDBOX__AS_TOKEN`; nested `__` env works). The
  namespace lives in a token-free TOML file selected by `TUWUNEL_CONFIG`.
- **Reset and deactivate need the current password.** Masquerading
  (`?user_id=`) is refused on `/account/password`, and an
  appservice-login device still gets the `m.login.password` UIA stage. The
  host therefore keeps each account's current password in the vault, at
  `<prefix>/cb_<hex>`. Vault names are limited to 64 characters; this was
  found against the live vault, so the prefix is at most 28. The vault is
  written before register. A failed rotation restores the previous value.
- **Contract.** It is exact to Cloudbox's `Provisioner`. `reset` on a missing
  account creates it, because Cloudbox's live E2E opens with `reset`. `ensure`
  on an existing account returns no password. A reset logs out all other
  sessions. The contract was posted to the host message board for the
  Cloudbox worker.
- **Placement.** The shim is a separate unit, `matrix-chat-cloudbox`
  (`examples/matrix-chat/cloudbox_shim.py`), exposed as outpost app
  `matrix-chat` with `require_login: false`. It forwards only the client,
  media and well-known roots. `bashy matrix-chat cloudbox CONFIG.json` wires
  it through existing `bashy service` and `bashy secret` mechanisms. No core
  command or Go change is needed.

## Gates and status

| gate | where | status |
|---|---|---|
| unit tests (`test_cloudbox_shim.py`, `test_provision.py`) | any host | PASS (16) |
| `cloudbox_gate.py`: signed tuwunel, real shim, synthetic test vault | dev host (darwin/arm64) | PASS (56 checks) |
| `cloudbox_gate.py` with `MATRIX_CHAT_GATE_VAULT=bashy` (production vault) | dev host | PASS (56 checks; refs removed) |
| `cloudbox_e2e.py`: live Cloudbox HTTPS, 3 users, outsider, revocation | disposable Cloudbox and chat host | **BLOCKED**: no test Cloudbox origin or tokens were available |
| `bashy matrix-chat cloudbox` under a running supervisord | manager chat host | not run (declarations verified with `bashy service add/verify` in an isolated fleet dir) |
