# Sprint 561 A3c: utility findings — date de_DE delta, man threshold, editor EOF diagnostic

Story `3219269cbc4c` (child of A3 `3d4d3d262bdd`, independent ownership; A3
acceptance unchanged). Scope: A2 I10 (date +34), I2 (man), I3 (EOF/editor).
Method per brief: standalone `GOWORK=off`, smallest causal repair at the
owning layer, red test first, no skips/weakened assertions/timeout
increases/new features. Baseline: frozen tree (bashy main == `5a4bd03f`
outside docs); this host Darwin/arm64, `go1.27.2`, GNU date 9.11 (`gdate`)
as oracle. Raw probe logs stay in `/tmp`, out of git.

## 1. Date de_DE.iso88591 delta — one product defect, fixed (C commit #1)

Repro: built `umbrella/coreutils/cmd/coreutils`, swept 50 strftime formats
x 5 fixed timestamps (`-d`, `TZ=UTC0`) under `LC_ALL=de_DE.ISO8859-1`,
ours vs GNU, byte-compared. Result: 250 cells, 50 diffs in 9 format
families: `%a %b %h %c %r %P %Ec %OY` (+ carried combos).

Root causes, split by oracle route:

- Real defects (logic, locale-independent): `%P` printed literally instead
  of lowercase `%p` (empty under de_DE, `pm` under C); `%E`/`%O` acceptance
  was POSIX-only, so GNU-rendered `%OY %Oz %Ep %EP %Ob %Op %Ob %Or %ON %OT
  %OR %Es %ET %ER %Eu %Ez %EZ %En %Et %OC %OG …` printed literally.
  Full GNU render sets were probed under C locale before fixing
  (`%E`: c C n p P r R s T t u x X y Y z Z; `%O`: b B C d e G H I m M n N
  p P r R s S t T U u V W w y Y z Z; everything else GNU passes through
  literally, and the fix matches that boundary exactly, minus `%q` which
  this applet does not implement).
- Host-libc corpus artifacts (NOT defects, NOT touched): `%a` (`Mi` vs
  macOS `Mi.`), `%b` (`Mär` vs macOS `März`), `%c` (glibc
  `%a %d %b %Y %T %Z` vs macOS zone-less form), `%r` (glibc `t_fmt_ampm`
  empty vs macOS non-empty). The embedded tables are the glibc de_DE
  corpus, corroborated twice: existing `date_test.go` pins (`Fr 06 Mär
  2026…`, `M\xe4r` Latin-1 bytes) and `cmds/locale/posixlocale.go`
  (`germanISO88591TimeKeywords`: abday without periods, abmon `M\xe4r`,
  `d_t_fmt "%a %d %b %Y %T %Z"`, `t_fmt_ampm ""`). The certification
  locale is glibc, so these already match there.

RED (before fix, `go test ./cmds/date/`):

    --- FAIL: TestDateGNUAlternativeModifierParity
    date +%p|%P|%OY|… = ("PM|%P|%OY|%OC|…", "", 0), want "PM|pm|2026|20|…"

Fix (`cmds/date/date.go`): added the missing `%P` case (new `amPM`
helper shared with `%p`); widened `eModified`/`oModified` to GNU's render
sets; documented the POSIX-vs-GNU boundary in the constant comment; listed
`%P` in the package `Supported directives` header. One pre-existing test
entry corrected to the verified oracle with cause recorded in place:
`TestDateAlternativeModifiers` pinned `%Oz` as literal, but GNU renders
`%Oz` as `%z` — it moved into that test's render table (which additionally
cross-checks modified-vs-unmodified output), it was not deleted.

GREEN (after fix): `GOWORK=off go test ./cmds/date/` — all 13 tests pass;
`go vet` clean; `gofmt` clean. Resweep: 50 → 40 diffs; all 40 remaining
are the host-libc corpus artifacts above (8 format groups x 5 ts).
Committed in `umbrella/coreutils` as `5c4e43b0`
(`fix(date): GNU %P and %E/%O parity under de_DE.iso88591`, trailers
Sprint #561 / Story #3054 / Story-ID 3219269cbc4c / Agent muse-conductor).

## 2. Man threshold failure — triaged, no defect, no code

A2 I2 is VSC TP `man:4`: the tset's own setup lists 72+37 command names
and requires `>109` before `man` runs; with `POSIX2_SW_DEV=undefined`
the count is exactly 109, so our `man` is never invoked (two independent
records: `S355-D-FIVE-TP-AUTHORITY-QUESTIONS-20261002.md`,
`S355-D-AUTHORITY-BATCH-INDEX-20261002.md`). The authority question
(configuration validity / PR treatment) is unsent and stays with the
approving authority. B-arm's separate missing-page failure ran against
GNU/system configuration, not our applet.

Local reproduction of everything the product controls: `man <name>` exits
0 with the JSON tool descriptor for all 153 registered applets, including
every preflight-required name (`alias bg cd command fc fg getopts jobs
unalias mailx talk`), shell builtins, and `gencat`/`getconf`. Only
`posix-gate` and `posix-providers` (deliberately excluded internal tools)
report "no documentation" — neither is a POSIX utility name. Per A2 "A3
only if a defect is confirmed": no defect confirmed, no commit. Note (not
a fix): `man 1 <name>` section form fails (`no documentation for 1`); the
POSIX synopsis is `man [-k] name...` and no tset evidence invokes our
applet with a section arg, so adding it would be a new feature — declined.

## 3. EOF/editor failures — bounded diagnostic only, no code

The four FAILs are `more:130,132,133,134` (ICs 801/803/804/805, locale
purposes): the licensed helper expects an editor interaction at final EOF
while the product exits 0 without invoking one. The editor-at-EOF product
patch stays reverted pending the required VSC authority answer
(`S355-D-MORE-OD-AUTHORITY-QUESTIONS-20261002.md` §"Confirmed EOF/editor
dependency"). Suite-free probes under an unchanged host PTY (`script(1)`,
marker `EDITOR`, no fixture changes), current coreutils main:

| Probe | Result |
|---|---|
| `v` before EOF (50-line file) | editor invoked with the file, exit 0 |
| `v` at final EOF, C locale (3-line file) | exit 0, editor NOT invoked |
| `v` at final EOF, `de_DE.UTF-8` | exit 0, editor NOT invoked |
| `q` under 8 locale spellings | all exit 0 except `de_DE.ISO8859-1` (dots, no dash) → exit 1, `more: LC_CTYPE: unsupported locale` |

This reproduces the record's mechanism exactly (editor dispatch before
EOF, clean exit 0 at final EOF — `pager.go:800-805` quits on any command
at last-file EOF). The spelling row is a latent inconsistency found
during diagnosis, reported not fixed: `more`'s `moreCharacterMode`
accepts `de_DE.iso88591` and `de_DE.ISO-8859-1` but rejects the dotted
`de_DE.ISO8859-1`, while `date` accepts all three. The record shows
licensed locale-search controls completing normally, so the helper
evidently uses an accepted spelling — cause unconfirmed, no code per the
brief. Confirmation needs the manager-controlled licensed replay with
pinned PTY/login fixture and tracing.

## 4. Pin bump + diffs for manager review (no merge, no push)

- Coreutils C commit (landed, unpushed): `5c4e43b0` in
  `/Users/qiangli/projects/poc/umbrella/coreutils` (main). Files:
  `cmds/date/date.go`, `cmds/date/date_test.go`,
  `cmds/date/locale_gate_test.go`. Nothing else in that checkout touched;
  `third_party/goawk` (live claim by genie-glm-5.3, contacted via family
  mailbox) and umbrella submodule pointers left alone.
- Bashy pin bump NOT applied by design: it requires the pushed commit to
  be resolvable, and pushing is prohibited. Manager steps after
  merge+push: `go get github.com/qiangli/coreutils@5c4e43b0` (moves
  `go.mod:20` and the `goawk` replace at `go.mod:539` together), then
  `GOWORK=off go test ./cmds/date/...` in coreutils and a bashy build.
- Compatibility evidence for the bump: with a temporary local
  `replace` (reverted afterwards, `go.mod` byte-identical),
  `GOWORK=off go build ./internal/core/` and `go vet ./internal/core/`
  both exit 0 against the fixed tree. `cmd/bashy` full link could not be
  attempted: host disk is 100% full (`link: … no space left on device`,
  environmental, unrelated to the change).
- `posixgate.go:26` stale comment NOT folded in: the brief's condition
  ("only if posixgate is otherwise touched") is false — posixgate was not
  touched. The 96/14/6, 88/22/6 values remain for whoever touches that
  file next.

## 5. Manager disposition (claude-opus5.5, 2026-11-04)

- **Date fix not merged; pin bump not merged.** The `%P` and widened
  `%E`/`%O` render sets are GNU extensions. POSIX `date` already renders
  its own `%E`/`%O` set, and this report's own sweep attributes every
  remaining de_DE difference to host-libc (macOS vs glibc) corpus
  artifacts that already match glibc on the Linux certification host. No
  link to a failing POSIX TP was shown, so the change is scope growth
  under the Sprint 341 base profile. Coreutils `5c4e43b0` is preserved
  unpushed on branch `s561/a3c-date-gnu-parity`; coreutils `main` is back
  at `601546e1`. I10 status: no POSIX defect reproduced on this host; the
  A4 Linux final-candidate run re-observes `date` under `de_DE.iso88591`.
- **I2 `man`:** accepted as triage. `man:4` never invokes the applet
  (109-name threshold with `POSIX2_SW_DEV=undefined`); authority question
  stays with the approving authority. `man 1 name` correctly declined.
- **I3 EOF/editor:** accepted as diagnostic. The four `more` results stay
  pending the VSC authority answer; no code. The `de_DE.ISO8859-1`
  spelling inconsistency in `more` is recorded, not fixed, until a TP is
  shown to depend on it.
