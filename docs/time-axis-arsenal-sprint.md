# Sprint: the Time Axis — bashy's temporal arsenal

In bashy's **space-time metaphor** (`docs/space-time-advisor.md`), almost every
command works the **space axis** — *where* data lives: files, dirs, hosts,
disk, network location (`ls cp find grep stat df du cd …`). A much smaller set
works the **time axis** — *when* things happen: the current instant, durations,
delays, deadlines, schedules, timezones, clock sync. Agents lean on the time
axis constantly (timeouts, poll-until-ready, "how long since…", "run at…"), yet
it's the thinnest part of the userland. This sprint fills it in — pure-Go,
cross-platform, self-contained, agent-first.

## 1. What bashy already has (the current time axis)

| Category | Present in bashy |
|---|---|
| Clock / timestamps | `date` (coreutils), `touch` (file mtime/atime), `stat` (shows a/m/c times); shell vars `EPOCHSECONDS`, `EPOCHREALTIME`, `SECONDS`, `TIMEFORMAT` |
| Duration / delay / deadline | `sleep` (GNU suffixes `2h`/`30m`), `timeout` (now native, this session), `time` (keyword + coreutils external), `times` (builtin), `wait` (builtin) |
| System time | `uptime` |
| Scheduling | **`bashy schedule`** — modern cron: `add` (`--cron/--interval/--every/--at/--once`), `list`, `rm`, `daemon` |

So bashy already covers *timestamps*, *durations/timeouts*, and *modern
scheduling*. The gaps are **calendar**, **periodic re-run**, **timezone
tooling**, **network-time/clock-skew**, **POSIX scheduling compat**, and a few
**agent-native duration verbs**.

## 2. Cross-platform time-command landscape (research)

Every category below, with the native command on each OS — the menu we pick
from. Bold = worth bringing into bashy (uniform, pure-Go-able, agentic).

| Category | Unix / Linux | macOS | Windows |
|---|---|---|---|
| Get/set clock | `date`, `timedatectl`, `hwclock` | `date`, `systemsetup -get/settime` | `date`/`time` (cmd), `Get-Date`/`Set-Date` |
| **Timezone** | `tzselect`, `zdump`, `TZ`, `timedatectl set-timezone`, `list-timezones` | `systemsetup -listtimezones` | `tzutil`, `Get-TimeZone`/`Set-TimeZone` |
| Delay | `sleep`, `usleep` | `sleep` | `timeout /t`, `Start-Sleep` |
| Deadline / duration | `timeout`, `time`, `times` | `timeout`, `time` | `Measure-Command`, `timeout` |
| Recurring schedule | `cron`/`crontab`, `anacron`, systemd `.timer` | `launchd`/`launchctl`, `cron` | `schtasks`, `Register-ScheduledTask` |
| One-shot schedule | `at`/`atq`/`atrm`/`batch` | `at` (atrun) | `schtasks /sc once`, `at` (deprecated) |
| **Periodic re-run** | `watch`, `entr` (on change) | `watch` (brew) | *(none native)* |
| **Network time (NTP)** | `ntpdate`, `sntp`, `chronyc`, `timedatectl set-ntp` | `sntp`, `systemsetup -setusingnetworktime` | `w32tm /resync /query` |
| **Calendar** | `cal`, `ncal`, `calendar`, `gcal` | `cal`, `ncal` | *(none native)* |
| Uptime / boot | `uptime`, `w`, `/proc/uptime` | `uptime`, `sysctl kern.boottime` | `Get-Uptime`, `systeminfo`, `net statistics` |
| Login times | `last`, `lastb`, `lastlog`, `who`, `w` | `last`, `who`, `w` | event logs |
| Process time | `time`, `/usr/bin/time -v`, `ps -o etime` | same | `Measure-Command` |
| Power / wake schedule | `rtcwake`, `systemctl suspend` | `caffeinate`, `pmset schedule` | `powercfg /waketimers`, `shutdown /t` |
| Benchmark | `time`, `hyperfine`, `perf stat` | same | `Measure-Command` |

**Key observation:** the same *concept* has a different command on each OS
(timezone = `tzselect`/`systemsetup`/`tzutil`; NTP = `sntp`/`w32tm`; periodic =
`watch`/nothing). bashy's edge is a **single uniform verb per concept** that
behaves identically on all three — exactly what it already does for `podman`,
`ollama`, `schedule`, etc.

## 3. Selection filter (bashy philosophy)

A time command earns a slot only if it is: **pure-Go** (no shelling out;
`time`, `time/tzdata`, `net` cover almost everything), **cross-platform**
(one behavior on Win/Mac/Linux), and **agentic-useful** (an agent actually
reaches for it). That rules *in* calendar/watch/tz/ntp/duration verbs; it rules
*out* (for now) hardware-clock, power/wake, and login-history commands, which
are OS-specific kernel/log surfaces best left to a download-exec or PATH tool.

Naming follows the house rule (`docs/…command-naming`): **classic POSIX names
stay short** (`cal`, `watch`, `at`, `crontab`); **new agentic capabilities get
descriptive composite verbs** (`tz`, `ntp`, `duration`) so they can't shadow a
classic namespace.

## 4. The sprint (scoped ~1 week; 3 shippable batches)

### Batch A — classic POSIX time tools (highest recognition, trivial pure-Go)

1. **`cal` / `ncal`** — calendar. `cal`, `cal 2026`, `cal 7 2026`, `-3`
   (prev/cur/next month), `-y`, today highlighted. Pure `time` package; ~a day
   incl. tests. Fills a gap present on Unix+macOS, absent on Windows — bashy
   makes it uniform.
2. **`watch`** — re-run a command every N seconds, repaint. `-n <secs>`,
   `-d` (highlight diffs), `-t` (no header), `-e`/`--errexit` (stop on
   non-zero), `-g`/`--chgexit` (exit on output change). Pure-Go, cross-platform
   — Windows has **no** native `watch`, so this is a genuine new capability.
   The agentic payoff is large: *poll-until-ready* (`watch -g curl -sf …`) is a
   core agent idiom. ~1–1.5 days.

### Batch B — agent-native, cross-platform time verbs (the differentiators)

3. **`tz`** — timezone toolkit (replaces `tzselect`/`zdump`/`systemsetup
   -listtimezones`/`tzutil`/`Get-TimeZone` with one verb). `tz list`,
   `tz now <zone>`, `tz convert "2026-07-05 14:30" America/New_York Asia/Tokyo`,
   `tz --json`. Pure-Go via `time.LoadLocation` + embedded `time/tzdata` (no
   system zoneinfo dependency → identical on Windows). ~1 day.
4. **`ntp`** (a.k.a. `sntp`) — pure-Go SNTP client. `ntp` (query a pool server,
   print the authoritative time + **offset/skew** vs the local clock), `ntp
   --check` (exit non-zero if skew > threshold), `ntp --server <host>`,
   `--json`. Agentic value: verify the clock before trusting timestamps / TLS /
   tokens; get a trusted `now` without root. Pure `net` UDP; no system NTP
   daemon. ~1 day.
5. **`duration`** — parse / format / humanize / arithmetic on durations and
   instants (no OS equivalent — a pure agent-ergonomics verb). `duration to-secs
   2h30m`, `duration humanize 9045` → `2h30m45s`, `duration since
   <epoch-or-ISO>`, `duration until "2026-07-05 18:00"`, `duration between A B`,
   `--json`. Token-lean answers to the "how long since/until/between" questions
   agents ask constantly. ~1 day.

### Batch C — POSIX scheduling compatibility shims (thin, map onto `schedule`)

6. **`at` / `atq` / `atrm` / `batch`** — one-shot scheduling. Thin front-ends
   that translate to `bashy schedule add --at …` / `list` / `rm`, so existing
   POSIX scripts (and the compliance suites) that call `at` just work. ~0.5 day.
7. **`crontab`** — `crontab -l/-e/-r/-` mapped onto the `schedule` store, so
   recurring-job scripts are portable to the bashy scheduler. ~0.5 day.

## 5. Backlog (deferred — fails the pure-Go/uniform filter)

- **Power/wake time**: `caffeinate` (macOS), `rtcwake` (Linux), `powercfg
  /waketimers` (Windows) — OS-specific kernel surfaces; revisit as
  platform-gated builtins or download-exec.
- **Clock *set* / hardware clock / NTP daemon**: `hwclock`, `timedatectl set-*`,
  `w32tm` service control — privileged, host-specific; out of scope for a
  self-contained userland (query-only `ntp` covers the read side).
- **Login/session history**: `last`, `lastlog`, `w` — parse OS-specific
  binary logs / `utmp` / Event Log; a separate "who's-been-here" effort.
- **Benchmark verb**: a `hyperfine`-style `bench` already has a dev-only
  precedent in `coreutils/cmds/perfbench`; productizing it is its own sprint
  (see `docs/bashy-agentic-performance-strategy.md`).

## 6. Acceptance / gate

Each new command: pure-Go + `CGO_ENABLED=0` cross-compiles to all six
platforms; unit tests; registered in `cmds/all` + surfaced by `bashy commands`;
`make test-bash` stays green. `cal`/`watch`/`at`/`crontab` additionally checked
for byte-compatible output against the GNU/BSD tools where a fixture exists.
No new runtime deps (embed `time/tzdata`; SNTP is stdlib `net`).

## 7. Effort summary

| Batch | Commands | Est. |
|---|---|---|
| A | `cal`, `watch` | ~2–2.5 d |
| B | `tz`, `ntp`, `duration` | ~3 d |
| C | `at`/`atq`/`atrm`/`batch`, `crontab` | ~1 d |
| **Total** | **9 commands / 4 verbs** | **~1 week** |

Ship order A → B → C: A is instant recognition + a real Windows gap filled;
B is the agentic differentiator; C is compat glue onto the scheduler bashy
already has.
