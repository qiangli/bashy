---
name: git
description: bashy dag front door for Git — build, one test script from t/, and the launched binary, as one dependency graph over the Makefile
default: smoke
vars:
  JOBS ?= 8
  TEST ?= t0000-basic.sh
---

# Git — task graph

The repo's own commands (`make`, a `t/t*.sh` test script, the built
`./git`), wired as a dependency graph so one front door replaces the
`INSTALL` + `t/README` incantations: `bashy dag --list` shows the targets,
`bashy dag run` builds git, then launches it. Lives at the repo root; needs
`bashy` (github.com/qiangli/bashy), a C compiler and the libraries
`Makefile` looks for on this host (zlib, curl, expat, gettext — the
`INSTALL` file lists them and the `NO_*` knobs that drop each). Everything
`make` writes is gitignored, so the tree stays clean.

## Tasks

### build
`make` — git and its helpers in the source tree (in-tree is git's own
layout; every product is gitignored). Minutes cold, seconds incremental;
`make` itself tracks what changed, so the target always runs it. Spelled
`env make` because bashy's in-process `make` is a POSIX make and git's
`Makefile` is GNU make's: `env` spawns the `make` on PATH, as it would
outside bashy.
Effects: read, write
Timeout: 1h

```bash
env make -j "$JOBS"
```

### test
One script from the test suite, run the way `t/README` says — from `t/`,
against the freshly built `./git`. `t0000-basic.sh` (the test framework's
own self-test, ~90 assertions) by default; `TEST=t3200-branch.sh` on the
command line picks another. The whole suite is `make test` — many minutes,
not a quick gate.
Requires: build
Effects: read, write

```bash
cd t && ./"$TEST"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~c` fence declares
`git()`, which reads the checkout's own coordinates — the `DEF_VER`
fallback version in `GIT-VERSION-GEN` and the number of built-in commands
the `Makefile` links into the binary (`BUILTIN_OBJS += builtin/…`) — and
hands one string back to the shell, which cross-checks it against the same
files. No build, no wrapper script, no `cc -o` quoting: the fence IS the
program. It is compiled once by the `cc`/`clang` on PATH (`BASHPP_CC`
overrides), C17, and runs as a fresh native process in the invoking
directory, so the relative paths are the checkout's.
Effects: read

```bsh
~~~c as c
#include <stdio.h>
#include <string.h>

static int read_version(char* out, size_t n) {
	FILE* f = fopen("GIT-VERSION-GEN", "r");
	if (!f) return -1;
	char line[256];
	out[0] = 0;
	while (fgets(line, sizeof line, f))
		if (strncmp(line, "DEF_VER=", 8) == 0) {
			snprintf(out, n, "%s", line + 8);
			out[strcspn(out, "\n")] = 0;
		}
	fclose(f);
	return out[0] ? 0 : -1;
}

static int count_builtins(void) {
	FILE* f = fopen("Makefile", "r");
	if (!f) return -1;
	char line[512];
	int n = 0;
	while (fgets(line, sizeof line, f))
		if (strncmp(line, "BUILTIN_OBJS += builtin/", 24) == 0) n++;
	fclose(f);
	return n;
}

const char* git(void) {
	static char out[256];
	char version[64];
	if (read_version(version, sizeof version) != 0) return "GIT-VERSION-GEN: no DEF_VER";
	snprintf(out, sizeof out, "git %s builtins %d", version, count_builtins());
	return out;
}
~~~
got := c.git()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
version=
while IFS= read -r line; do
	case "$line" in DEF_VER=*) version=${line#DEF_VER=} ;; esac
done <GIT-VERSION-GEN
builtins=0
while IFS= read -r line; do
	case "$line" in "BUILTIN_OBJS += builtin/"*) builtins=$((builtins + 1)) ;; esac
done <Makefile
want="git $version builtins $builtins"
[ "$got" = "$want" ] || { echo "smoke: c.git() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built binary from a Bash++ body: the `~~~c` fence's `launch()`
runs `./git --version` through `popen` and returns its first line; the
shell checks it names the release `GIT-VERSION-GEN` declares (`v2.56.0-rc0`
→ `git version 2.56.0…`; the suffix is `git describe`'s). The fence is the
launcher, the `build` dependency guarantees the binary exists first.
Requires: build
Effects: read

```bsh
~~~c as c
#include <stdio.h>
#include <string.h>

const char* launch(void) {
	static char line[512];
	FILE* p = popen("./git --version", "r");
	if (!p) return "popen: ./git";
	if (!fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) return "./git --version: non-zero exit";
	line[strcspn(line, "\n")] = 0;
	return line;
}
~~~
got := c.launch()
version=
while IFS= read -r line; do
	case "$line" in DEF_VER=*) version=${line#DEF_VER=v}; version=${version%%-*} ;; esac
done <GIT-VERSION-GEN
case "$got" in
	"git version $version"*) ;;
	*) echo "run: c.launch() -> '$got', want 'git version $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
