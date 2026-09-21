---
name: ffmpeg
description: bashy dag front door for FFmpeg — configure, the ffmpeg binary, one FATE test and the launched CLI, as one dependency graph over the Makefile
default: smoke
vars:
  JOBS ?= 8
  CONFIGURE_OPTS ?= --disable-doc
  FATE ?= fate-source
---

# FFmpeg — task graph

The repo's own commands (`./configure`, `make ffmpeg`, `make fate-…`, the
built `./ffmpeg`), wired as a dependency graph so one front door replaces
the `INSTALL.md` / `doc/build_system.txt` walkthrough: `bashy dag --list`
shows the targets, `bashy dag run` configures, builds the CLI, then
launches it. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy), a C compiler, GNU make and, on x86, `nasm`
(`--disable-x86asm` otherwise — `configure` says so). FFmpeg builds
in-tree and gitignores every product, so the tree stays clean.

## Tasks

### configure
`./configure` — `--disable-doc` by default (no texinfo needed); other
switches go in `CONFIGURE_OPTS`. Writes `config.h`, `config.mak` and the
per-library `config_components.h` into the tree (all gitignored). Re-run
only when `configure` itself changes; a minute the first time.
Sources: configure ffbuild/version.sh
Generates: config.h config.mak
Effects: read, write
Timeout: 30m

```bash
./configure $CONFIGURE_OPTS
```

### build
The `ffmpeg` binary and the libraries it links (`make ffmpeg`). Minutes
cold, incremental after; `make` tracks what changed, so the target always
runs it. Spelled `env make` because bashy's in-process `make` is a POSIX
make and FFmpeg's `Makefile` is GNU make's: `env` spawns the `make` on
PATH, as it would outside bashy.
Requires: configure
Effects: read, write
Timeout: 1h

```bash
env make -j "$JOBS" ffmpeg
```

### test
One FATE target, run the way `tests/fate.sh` and `doc/fate.texi` do:
`make fate-<name>`. `fate-source` by default — the source-tree checks
(license headers, forbidden constructs) that need no samples; `FATE=fate-h264`
on the command line picks another (most need `FATE_SAMPLES`, see
`doc/fate.texi`). The whole suite is `make fate` — hours, not a quick gate.
Requires: configure
Effects: read, write

```bash
env make -j "$JOBS" "$FATE"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~c` fence declares
`ffmpeg()`, which reads the checkout's own coordinates — the `RELEASE`
file, the `LIBAVUTIL_VERSION_{MAJOR,MINOR,MICRO}` macros in
`libavutil/version.h` (read as text: the header needs `configure`'s
generated `avconfig.h` to compile, so before a build it is data, not an
include) and the number of `libav*`/`libsw*` libraries — and hands one
string back to the shell, which cross-checks it against the same files. No
build, no wrapper script, no `cc -o` quoting: the fence IS the program. It
is compiled once by the `cc`/`clang` on PATH (`BASHPP_CC` overrides),
C17, and runs as a fresh native process in the invoking directory, so the
relative paths are the checkout's.
Effects: read

```bsh
~~~c as c
#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int macro_value(const char* path, const char* name) {
	FILE* f = fopen(path, "r");
	if (!f) return -1;
	char line[256];
	size_t n = strlen(name);
	int value = -1;
	while (fgets(line, sizeof line, f))
		if (strncmp(line, "#define ", 8) == 0 && strncmp(line + 8, name, n) == 0 && line[8 + n] == ' ')
			value = atoi(line + 8 + n);
	fclose(f);
	return value;
}

static int count_libraries(void) {
	DIR* d = opendir(".");
	if (!d) return -1;
	int n = 0;
	struct dirent* e;
	while ((e = readdir(d)))
		if (strncmp(e->d_name, "libav", 5) == 0 || strncmp(e->d_name, "libsw", 5) == 0) n++;
	closedir(d);
	return n;
}

const char* ffmpeg(void) {
	static char out[256];
	char release[64] = "";
	FILE* f = fopen("RELEASE", "r");
	if (!f) return "RELEASE: unreadable";
	if (fgets(release, sizeof release, f)) release[strcspn(release, "\n")] = 0;
	fclose(f);
	snprintf(out, sizeof out, "ffmpeg %s avutil %d.%d.%d libraries %d", release,
		macro_value("libavutil/version.h", "LIBAVUTIL_VERSION_MAJOR"),
		macro_value("libavutil/version.h", "LIBAVUTIL_VERSION_MINOR"),
		macro_value("libavutil/version.h", "LIBAVUTIL_VERSION_MICRO"),
		count_libraries());
	return out;
}
~~~
got := c.ffmpeg()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
macro() { # <name> <file>: the token after `#define <name>`
	name=$1
	while IFS= read -r line; do
		case "$line" in "#define $name "*) set -- $line; printf '%s' "$3"; return 0 ;; esac
	done <"$2"
	return 1
}
IFS= read -r release <RELEASE
libraries=0
for d in libav* libsw*; do [ -d "$d" ] && libraries=$((libraries + 1)); done
want="ffmpeg $release avutil $(macro LIBAVUTIL_VERSION_MAJOR libavutil/version.h).$(macro LIBAVUTIL_VERSION_MINOR libavutil/version.h).$(macro LIBAVUTIL_VERSION_MICRO libavutil/version.h) libraries $libraries"
[ "$got" = "$want" ] || { echo "smoke: c.ffmpeg() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: after `build`, the checkout
carries the generated `libavutil/ffversion.h`, so the `~~~c` fence
`#include`s it — the checkout root is an include root for the fence — and
`launch()` runs `./ffmpeg -version` through `popen`, returning the
compile-time `FFMPEG_VERSION` and the binary's first output line; the
shell checks the two agree. The fence is the launcher, the `build`
dependency guarantees both the header and the binary exist first.
Requires: build
Effects: read

```bsh
~~~c as c
#include <stdio.h>
#include <string.h>
#include "libavutil/ffversion.h"

const char* launch(void) {
	static char out[512];
	char line[256];
	FILE* p = popen("./ffmpeg -version", "r");
	if (!p) return "popen: ./ffmpeg";
	if (!fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) return "./ffmpeg -version: non-zero exit";
	line[strcspn(line, "\n")] = 0;
	snprintf(out, sizeof out, "%s|%s", FFMPEG_VERSION, line);
	return out;
}
~~~
got := c.launch()
header=${got%%|*}
line=${got#*|}
case "$line" in
	"ffmpeg version $header "*) ;;
	*) echo "run: c.launch() -> '$got', want 'ffmpeg version $header …'" >&2; exit 1 ;;
esac
echo "run: $line"
```
