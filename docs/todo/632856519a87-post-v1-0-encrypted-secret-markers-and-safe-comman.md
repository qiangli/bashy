---
id: 632856519a87
kind: task
title: 'Post-v1.0: encrypted secret markers and safe command-boundary injection'
seq: 5
status: todo
priority: p2
created: 2026-08-11T14:29:48.740621Z
---

Post-v1.0 only. Do not preempt the Bash 5.3 / POSIX / v1.0 release-critical work.

Goal: let Bashy carry an encrypted marker instead of raw secret text through supported
command surfaces, then decrypt only at the last safe command boundary, so the value does
not land in shell history, argv captures, PTY logs, agent transcripts, or copy-pasted
work notes as PLAINTEXT.

This is three layers, not one:

- encrypted-at-rest / in-transit marker text;
- safe runtime materialization at a specific command boundary;
- output/capture redaction as defense in depth.

Do not collapse them. A target process will still see plaintext in memory, and a process
that intentionally prints or exfiltrates it can still leak it. This feature reduces
accidental disclosure and the routine log/console/argv/proc class of leaks; it is not a
claim of end-to-end secrecy against a compromised host, root/admin, or a malicious target
binary.

Shellph prior art: useful UX inspiration, not reusable crypto. As of
2026-07-20 (`44810be`, MIT), <https://github.com/xirtam2669/Shellph> offers AES-CBC, RC4,
and XOR transforms, with README-default key/IV material `1234567890123456`, and its code
path is encryption/formatting oriented rather than an authenticated decrypt-at-boundary
secret-consumption system. Bashy must NOT copy RC4, XOR, fixed-IV CBC, or any unauthenticated
envelope from it.

Threat model:

- prevent accidental plaintext disclosure into shell history, `ps`/argv views, `/proc`
  command-line inspection, audit logs, PTY captures, agent transcripts, and copied notes;
- prevent operator/agent mistakes where a supported tool would otherwise receive a raw
  password on argv/stdin and durable captures later preserve it;
- preserve enough metadata to say "this is a Bashy secret marker" and route it through the
  correct decryptor without heuristic guessing.

Non-goals:

- hiding plaintext from the destination process after Bashy hands it over;
- replacing `bashy secrets` (cloudbox vault), `bashy ask`, or the output firewall;
- making unsupported third-party tools magically safe;
- promising secrecy against host compromise, same-user replay from unrestricted local
  storage, or intentional transform-and-exfiltrate attacks.

Proposed marker format (design target, not yet verb/API-final):

```text
bshsec:v1:<base64url(envelope)>
```

Where `envelope` starts with an INNER magic/version header so accidental prefix collisions
still fail closed after decode:

- magic: `BSY1`
- version: `1`
- alg: `c20p` (ChaCha20-Poly1305)
- `kid`: key identifier for rotation
- flags: portable/local-only, generic/boundary-bound, replay policy
- `scope`: canonical consumer binding, e.g. `adapter:chpasswd`, `adapter:ssh-password`,
  or `generic:text`
- `ctx_hash`: optional context binding hash (for example normalized target tuple such as
  `user@host`, or a caller-supplied generic context string)
- `salt`
- `nonce`
- ciphertext+tag

Key schedule target:

- store one or more local master keys by `kid`;
- derive a per-envelope content key with HKDF-SHA-256 from `master_key[kid] + salt`;
- seal with AEAD using AAD over the version / kid / flags / scope / ctx binding;
- base64url without padding for shell-safe single-token transport;
- decrypt must fail closed on wrong `kid`, tamper, truncation, scope mismatch, context
  mismatch, or backend unavailability, and MUST NOT emit partial plaintext.

Reasoning:

- AEAD, not encrypt-then-pray: see RFC 5116 and RFC 8439;
- explicit AAD binding lets the same local secret material refuse the wrong adapter;
- version + `kid` make rotation and future algorithm changes possible without silent
  breakage;
- human-visible prefix + inner magic make detection unambiguous and cheap.

Default key custody order:

1. macOS Keychain generic-password item (`SecKeychainAddGenericPassword` /
   `SecKeychainFindGenericPassword`)
2. Linux Secret Service item/session backend
3. Windows DPAPI user-scoped protection (`CryptProtectData`)
4. Explicit local `0600` fallback key file ONLY when no stronger backend is available,
   with a loud `doctor` / command warning and no claim of same-host theft resistance

Do not require cloudbox pairing for this. The feature must still work on standalone hosts.
But when paired, it should compose with `bashy secrets` so a vault value can be wrapped
locally for later safe reuse without retyping.

Runtime materialization rule of record:

Never substitute a decrypted secret into argv when a safer sink exists.

Per-adapter sink preference:

1. anonymous pipe / inherited FD / child stdin
2. private `0600` temp file path or one-shot helper path
3. target-specific askpass / prompt bridge / PTY shim
4. environment variable only when the target protocol genuinely requires it
5. argv: REFUSE by default; allow only behind an explicit insecure escape hatch that
   names the risk

First supported boundary tranche after v1.0:

- `chpasswd`-style stdin consumers: if input contains `user:bshsec:v1:...`, Bashy
  decrypts only while streaming to the child stdin pipe, never to logs or argv
- `ssh` password/passphrase flows: prefer existing key/agent auth; if password bridging
  is implemented, use an askpass or PTY/FD bridge rather than command-line password
  injection
- generic explicit helpers for "supported command wants secret on stdin/file/env"
  rather than broad magical rewrites across all tools

Dependencies / composition rules:

- `docs/shell-secrets-vault-design.md`: durable named secrets still live there
- `docs/bashy-ask-human-input-design.md`: ad-hoc human capture still lands there first
- `docs/secret-output-firewall.md`: must redact known plaintext VALUES on display/persist
  paths, and should also recognize `bshsec:v1:` handles as sensitive replayable references
  in agent logs/transcripts

Important design boundary: an encrypted marker is NOT the same thing as safe injection.
Logging `bshsec:v1:...` is better than logging plaintext, but on a host that can decrypt it
the marker is still a replayable capability. Treat it as sensitive durable output.

Migration / phases:

M0 — research/design (this task)

- settle envelope shape, backend order, and failure semantics
- settle whether the local-only default binds to host+uid, and what `--portable` means
- settle verb naming without overloading the existing cloudbox `bashy secrets` UX

M1 — explicit primitives

- add seal/unseal/rewrap primitives for markers
- add local key management + rotation by `kid`
- refuse `unseal --stdout` unless the user explicitly accepts transcript risk

M2 — generic safe injection helpers

- file/FD/stdin/env adapters with one machine-readable capability contract
- integration tests proving plaintext never appears on parent argv/stdout/stderr

M3 — first built-in command adapters

- `chpasswd`
- one SSH password/passphrase bridge
- at least one additional stdin/file consumer chosen from real fleet usage

M4 — redaction and audit follow-through

- output firewall recognizes marker handles and plaintext descendants
- audit command can scan logs/transcripts for both known plaintext values and extant
  marker handles

Acceptance tests:

- marker parse is exact: plaintext that merely resembles the prefix must not be silently
  consumed
- tamper, truncation, nonce corruption, wrong key, wrong scope, wrong context, and missing
  backend all fail closed with no plaintext output
- key rotation: old `kid` decrypts until rewrap; new seals use the new `kid`
- `chpasswd` adapter proves plaintext appears only on child stdin, not in parent argv,
  env, stderr, stdout, or PTY capture
- SSH bridge proves no command-line password injection and no transcript plaintext
- output firewall redacts both a direct plaintext secret and any accidental marker echo in
  durable capture paths
- regression test for split writes / chunk boundaries on redaction remains green

Misuse risks to design against:

- "encrypt then pass on argv anyway" — defeats the point; forbid by default
- "generic unseal to stdout" — transcript footgun; require explicit unsafe opt-in
- "context-free portable markers everywhere" — easier copy/paste, worse replay risk;
  default should be locally bound unless the operator asks otherwise
- "just use env" — GitHub and procfs/systemd docs all warn that env/cmdline are still
  exposure surfaces
- "Shellph but safer later" — no; authenticated, rotatable, bound envelope is the floor

Research notes / primary sources checked 2026-08-11:

- Shellph repo + source: <https://github.com/xirtam2669/Shellph>
- age spec: header MAC, recipient stanzas, versioned format:
  <https://age-encryption.org/v1>
- AEAD interface / nonce guidance: <https://www.rfc-editor.org/rfc/rfc5116>
- ChaCha20-Poly1305 AEAD: <https://www.rfc-editor.org/rfc/rfc8439>
- systemd credentials: credentials on `/proc/cmdline` are visible and should be avoided;
  file/path-style credential propagation is the model:
  <https://systemd.io/CREDENTIALS/>
- Linux `/proc/<pid>/cmdline`: argv is inspectable process memory:
  <https://man7.org/linux/man-pages/man5/proc_pid_cmdline.5.html>
- Linux `/proc/<pid>/environ`: exec-time environment is inspectable:
  <https://man7.org/linux/man-pages/man5/proc_pid_environ.5.html>
- OpenSSH askpass / batch mode semantics:
  <https://man.openbsd.org/ssh>
  <https://man.openbsd.org/ssh_config>
- `chpasswd` consumes `user:password` on stdin and warns about cleartext files:
  <https://man7.org/linux/man-pages/man8/chpasswd.8.html>
- GitHub Actions secret masking guidance and warning against command-line secret passing:
  <https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets>
- Secret Service API draft:
  <https://specifications.freedesktop.org/secret-service-spec/latest/>
- Windows DPAPI user-scoped protection:
  <https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata>

Deliverable for the implementation sprint: a post-v1.0 feature that makes the SAFE path
easier than raw plaintext, proves where plaintext can and cannot exist, and composes with
the existing vault / ask / output-firewall work instead of competing with it.
