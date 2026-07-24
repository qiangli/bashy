.PHONY: dag build build-bash build-bashy install test test-bash test-bash-run test-bash-parallel test-bash-list test-bash-helpers dist tidy clean help

BIN_DIR := bin
BIN := $(BIN_DIR)/bashy
BASH_TESTS_DIR := external/bash-5.3/tests
# The bash test fixtures invoke the shell as `bash` / via $BASH, so the
# compliance harness drives a copy named `bin/bash`.
BASHY := $(BIN_DIR)/bash

# Stamp a real version onto release builds. Override on the command line, e.g.
#   make build VERSION=v0.1.0
VERSION ?= dev
BUILD_ID ?= $(shell if [ -e .git ] && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		id=$$(git describe --tags --exact-match HEAD 2>/dev/null || git rev-parse --short=7 HEAD 2>/dev/null); \
		if [ -n "$$id" ]; then \
			if ! git diff --quiet --ignore-submodules -- 2>/dev/null || ! git diff --cached --quiet --ignore-submodules -- 2>/dev/null; then \
				id="$$id-dirty"; \
			fi; \
			printf '%s' "$$id"; \
		fi; \
	fi)
# -s -w strip the symbol table and DWARF debug info; with -trimpath (below)
# this drops the binary ~30% (≈7.8M → ≈5.4M). A pure-Go bash can't reach C
# bash's ~1.2M — the Go runtime/GC (~2.3M) plus the interpreter and the
# x/text CJK charset tables (Big5/Shift-JIS, needed for locale-correct globs)
# set a floor around 5M.
LDFLAGS := -s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-$(VERSION)' -X 'github.com/qiangli/bashy/internal/cli.buildID=$(BUILD_ID)'

# The Go FIPS 140-3 module version selected by the build-fips target (see
# `go tool` / go.dev/doc/security/fips140). v1.0.0 holds CMVP certificate #5247.
GOFIPS140_VERSION ?= v1.0.0

# Platforms for `make dist` (goreleaser handles real releases; this is a
# local cross-compile sanity check).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Build profile (cmd/bashy only — cmd/bash is always the pure drop-in). The
# DEFAULT is the lean worker — shell + coreutils userland + git + dag + `bashy go`
# — which cross-compiles to EVERY platform with CGO_ENABLED=0 (this is what gets
# released). Two opt-in, unix-only, heavier host layers:
#   BASHY_ENGINES=1  container/LLM engines (bashy podman/ollama) + their embedded
#                    helper blobs when present (podman/vfkit/gvproxy .gz built by
#                    coreutils/scripts/embed-*.sh). cgo, btrfs/MLX — unix only.
#   BASHY_OBS=1      observability stack (bashy otel): ~193 MB of OpenTelemetry
#                    Collector + VictoriaMetrics/Logs + Jaeger + Perses + k8s/aws.
# `make build-host` turns on both.
EMBED_DIR := ../coreutils/external/podman/engine
ENGINE_TAGS := $(if $(BASHY_ENGINES),bashy_engines \
	$(if $(wildcard $(EMBED_DIR)/podman_embed/podman.gz),embed_podman) \
	$(if $(wildcard $(EMBED_DIR)/vfkit_embed/vfkit.gz),embed_vfkit) \
	$(if $(wildcard $(EMBED_DIR)/gvproxy_embed/gvproxy.gz),embed_gvproxy))
BASHY_TAGS := $(strip $(ENGINE_TAGS) $(if $(BASHY_OBS),bashy_obs))

## dag: Bootstrap/run the repo-local DAG runner. Pass ARGS="build", ARGS="test", etc.
dag:
	@./bashy dag $(if $(ARGS),$(ARGS),--list)

## build: Build both independent binaries into bin/ (bash = pure drop-in from
## cmd/bash; bashy = AgentOS shell from cmd/bashy). They share the cli core but
## are separate compilations — bash's import graph never includes coreutils.
## Default is the LEAN worker; use `make build-host` for the full unix host shell.
build: build-bash build-bashy

## build-host: Full unix host bashy — engines (bashy podman/ollama, + embed blobs
## if present) and the observability stack (bashy otel). Not cross-platform.
build-host:
	$(MAKE) build BASHY_ENGINES=1 BASHY_OBS=1

## build-bash: Build only the pure drop-in (cmd/bash -> bin/bash). This is all
## the conformance harness needs; it skips the embed-heavy bin/bashy build.
build-bash:
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BASHY) ./cmd/bash

## build-bashy: Build the AgentOS shell (cmd/bashy -> bin/bashy), embedding the
## podman engine blobs when present (large binary; not needed for test-bash).
build-bashy:
	@echo "building bashy$(if $(BASHY_TAGS), with embeds: $(BASHY_TAGS),) ..."
	go build -trimpath $(if $(BASHY_TAGS),-tags "$(BASHY_TAGS)",) -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/bashy

## build-fips: Build both binaries against the Go FIPS 140-3 validated crypto
## module (CMVP #5247). Run with GODEBUG=fips140=on for FedRAMP/CMMC/gov use.
## Do NOT use fips140=only for a general shell — it rejects MD5 (breaks md5sum).
## Pure-Go, CGO_ENABLED=0: no BoringCrypto, no OpenSSL, no cgo.
build-fips:
	@mkdir -p $(BIN_DIR)
	@echo "building with the Go FIPS 140-3 module (GOFIPS140=$(GOFIPS140_VERSION)) ..."
	GOFIPS140=$(GOFIPS140_VERSION) go build -trimpath -ldflags "$(LDFLAGS)" -o $(BASHY) ./cmd/bash
	GOFIPS140=$(GOFIPS140_VERSION) go build -trimpath $(if $(BASHY_TAGS),-tags "$(BASHY_TAGS)",) -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/bashy

## install: Build and atomically install both binaries into the shared dhnt user
## bin ($$DHNT_BIN_DIR, default $$HOME/.local/bin). The installer refuses
## binaries missing the core AgentOS command surface.
install: build
	go run ./tools/installbashy -bash $(BASHY) -bashy $(BIN)

## test: Run all Go tests
test:
	go test ./...

## dist: Cross-compile static binaries for all release platforms into bin/dist/
## (both bash and bashy; goreleaser handles real releases, this is a local
## cross-compile sanity check).
dist:
	@mkdir -p $(BIN_DIR)/dist
	@for plat in $(PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; \
		ext=; [ "$$os" = windows ] && ext=.exe; \
		for name in bash bashy; do \
			out=$(BIN_DIR)/dist/$$name-$$os-$$arch$$ext; \
			echo "building $$out..."; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build -trimpath -ldflags "$(LDFLAGS)" -o $$out ./cmd/$$name || exit 1; \
		done; \
	done

BASH_TEST_TIMEOUT := 60
# jobs runs a long sequence of real backgrounded sleeps (job-control timing);
# it needs more than the default per-test cap even with working `kill` reaping.
BASH_TEST_TIMEOUT_JOBS := 120
# Per-fixture memory cap (KB) enforced by scripts/memwatch.sh. macOS does NOT
# honor `ulimit -v`, so an unbounded-allocation fixture (e.g. intl/unicode1.sub)
# can balloon to 100+ GB before the wall-clock timeout fires. The watchdog
# SIGKILLs the fixture's process group past this RSS, turning an OOM into a
# graceful fixture failure. 4 GB is far above any legitimate fixture.
BASH_TEST_MEM_KB := 4194304

# NOTHING is skipped — the full bash-5.3 suite passes (86/86). Each fixture was
# closed by matching bash 5.3 EXACTLY (inspect the reference before calling a
# fixture a "ceiling"):
# (coproc — coproc lifecycle: synthetic per-runner PID so wait/kill $COPROC_PID
#  resolve, signal-death status, fd reuse/close→-1.)
# (glob-test — byte-transparent per LC_CTYPE: $'\u' encodes in the locale charset
#  (u32cconv), the lexer treats invalid/incomplete multibyte as opaque single
#  bytes (MB_INVALIDCH→1, never errors), read/IFS split per MB_CUR_MAX.)
# (trap — startup-ignored signals can't be re-trapped; SIGCHLD trap fires once
#  per reaped child (jobs.c:waitchld).)
# (execscript — exec exit codes 126/127, command_not_found_handle, exec/`.`-on-
#  directory wording, EXIT-trap-in-subshell, BASH_SUBSHELL, expand_aliases.)
# (jobs — real process-group job control on unix (setpgid + Wait4 WUNTRACED
#  stopped-state + kill -STOP/-CONT + fg/bg + suspend messages), all in sh's
#  *_unix.go; needs the longer BASH_TEST_TIMEOUT_JOBS above. Mirrors bash's own
#  jobs.c (unix) / nojobs.c (elsewhere) split.)
BASH_TEST_SKIP :=

# Tests whose bash run-* helper strips lines starting with `expect ` from
# the captured output before diffing against the .right file. The
# convention is local to a handful of tests: most embed `expect` echoes
# directly in the .right file (so filtering them would break the diff).
BASH_TEST_FILTER_EXPECT := attr exp exp-tests extglob extglob2 invert invocation more-exp new-exp nquote nquote1 nquote2 nquote3 nquote5 posix2 varenv

# Tests whose bash run-* helper pipes captured output through `cat -v` to
# make control characters visible (NUL -> ^@, BEL -> ^G, ESC -> ^[, etc.)
# before diffing against the .right file. Apply the same transform here
# so raw control bytes don't trip the byte-for-byte diff.
BASH_TEST_CAT_V := printf

# The upstream test.tests fixture assumes /tmp allows setuid/setgid bits
# and that fd 0 is a terminal. Normalize only those host-dependent lines
# below so the fixture still checks bashy's test builtin behaviour.

## test-yash: yash POSIX (-p) conformance scoreboard — the yash analogue of test-bash.
## Runs every shell-agnostic *-p.tst against bashy AND real bash in one container,
## per testcase, and lists the BASHY-SPECIFIC failures (bash passes, bashy fails) —
## the genuine bugs to fix. Job-control/signal suites excluded (goroutine ceiling).
## Output dir via YASH_OUT (default /tmp/yash-scoreboard); failures in <dir>/failures.txt.
test-yash:
	@scripts/yash-scoreboard.sh $(YASH_OUT)

## test-yash-list: print the current bashy-specific yash failure list (suite line desc).
test-yash-list: test-yash
	@cat $${YASH_OUT:-/tmp/yash-scoreboard}/failures.txt

## test-zsh: zsh-own-suite scoreboard (Tier 0 of the zsh ladder) — runs zsh 5.9's
## Test/*.ztst (non-interactive classes A B C D E W Z) against bashy AND real zsh
## through the same runner (tools/ztst); real zsh defines the valid denominator.
## INFO metric, not a gate. Output dir via ZSH_OUT (default /tmp/zsh-scoreboard).
test-zsh:
	@scripts/zsh-scoreboard.sh $(ZSH_OUT)

## test-zsh-list: print the current zsh-own-suite failure list (file:line desc).
test-zsh-list: test-zsh
	@cat $${ZSH_OUT:-/tmp/zsh-scoreboard}/failures.txt

## test-uutils: uutils test-suite scoreboard — runs the MIT-licensed
## uutils/coreutils test suite (via cargo + its UUTESTS_BINARY_PATH override)
## against the pure-Go coreutils multicall from ../coreutils (the same tool
## registry bashy mounts in-process). INFO metric, not a gate — many cases
## assert uutils-specific diagnostics. Needs cargo + the gitignored clone at
## ../coreutils/reference/uutils-coreutils. Output dir via UUTILS_OUT.
test-uutils:
	@scripts/uutils-scoreboard.sh $(UUTILS_OUT)

## test-uutils-list: print the current uutils-suite failure list (module::case).
test-uutils-list: test-uutils
	@cat $${UUTILS_OUT:-/tmp/uutils-scoreboard}/failures.txt

## smoke-chat: drive `bashy chat` interactive under a real pty against an installed
## agent — asserts the governed-launcher contract (native launch · registry · steer
## · capture tee · teardown). INFO, never a gate: SKIPs cleanly without an agent or
## a pty (headless CI). Pass an agent as the first arg: make smoke-chat AGENT=codex-gpt-5.5.
smoke-chat:
	@scripts/chat-smoke.sh $(AGENT)

## test-bash: Run bash 5.3 native test suite against bashy (with per-test timeout).
## Builds only the lean bin/bash drop-in (not the 259MB embed-heavy bin/bashy).
## Iterate fast on a subset with TESTS="name ...", e.g. make test-bash TESTS="comsub varenv".
test-bash: build-bash test-bash-helpers
	@$(MAKE) --no-print-directory test-bash-run

## test-bash-run: the fixture loop only (no build). Used by `test-bash` (which
## builds first) and by scripts/test-bash-parallel.sh (builds once, then fans
## the loop out over fixture groups). Honors TESTS="name ..." like test-bash.
test-bash-run: $(BIN_DIR)/bash53suite
	@BASH53_TIMEOUT=$(BASH_TEST_TIMEOUT)s \
	 BASH53_JOBS_TIMEOUT=$(BASH_TEST_TIMEOUT_JOBS)s \
	 BASH53_MEM_KB=$(BASH_TEST_MEM_KB) \
	 $(BIN_DIR)/bash53suite -tests-dir $(BASH_TESTS_DIR) -bash $(BASHY) \
	   -tests "$(TESTS)" -skip "$(BASH_TEST_SKIP)"

## bin/bash53suite: the ONE fixture runner. `bashy dag` drives the same binary,
## so `make test-bash` and a chunked/distributed dag run are the same program —
## which is what makes "chunked == serial" a checkable claim rather than a hope.
$(BIN_DIR)/bash53suite: tools/bash53suite/*.go
	@mkdir -p $(BIN_DIR)
	@go build -o $@ ./tools/bash53suite

# --- retired: the shell fixture loop -----------------------------------------
# Until 2026-07-12 this file implemented a SECOND fixture runner in shell: a
# per-fixture background job, a `sleep N && kill -KILL -- -$pid` watchdog, a
# memwatch.sh sidecar, and the .right diffing. It is gone, and its guards live
# in tools/bash53suite (skip list, 4GB memory cap, HOME isolation, the expect /
# cat -v transforms, the whole-suite deadline).
#
# Why it had to go, not just be tidied:
#
#  1. Its watchdog was broken. `kill -KILL -- -$test_pid` targets a PROCESS
#     GROUP that only exists if the testee honored BASH_SETPGRP, and the kill
#     was `2>/dev/null`, so when that assumption failed the watchdog failed
#     SILENTLY and `wait` blocked forever. That is the 20-minute CI hang. The Go
#     harness sets Setpgid from the PARENT, so the testee's cooperation is
#     irrelevant, and it always prints a Results line.
#  2. Two runners meant "chunked vs serial" compared two different PROGRAMS, so
#     the equality property that makes distributed conformance trustworthy was
#     vacuous. A trap regression (86/86 -> 85/86) rode into main because the one
#     gate that would have caught it was the one that hung.
#
# The old loop, for the record (do not resurrect):
#   for runner in run-*; do ... & test_pid=$!; \
#     ( sleep $per_test_timeout && kill -KILL -- -$test_pid 2>/dev/null ) & \
#     sh scripts/memwatch.sh $test_pid $(BASH_TEST_MEM_KB) & \
#     wait $test_pid; ... diff $BASH_TSTOUT $right_file ...
# -----------------------------------------------------------------------------
.PHONY: test-bash-run-legacy
test-bash-run-legacy:
	@echo "the shell fixture loop was retired 2026-07-12; use test-bash-run (see Makefile)" >&2
	@exit 2

## test-bash-parallel: Run the bash 5.3 suite in parallel fixture groups (builds
## bin/bash once, then fans the loop out over JOBS groups). JOBS defaults to the
## CPU count; on a big box use e.g. `make test-bash-parallel JOBS=20`.
test-bash-parallel: build-bash test-bash-helpers
	@JOBS=$(JOBS) BASH_TESTS_DIR=$(BASH_TESTS_DIR) BASH_TEST_SKIP="$(BASH_TEST_SKIP)" /bin/bash scripts/test-bash-parallel.sh

## test-bash-list: List all available bash 5.3 tests
test-bash-list: $(BIN_DIR)/bash53suite
	@$(BIN_DIR)/bash53suite -tests-dir $(BASH_TESTS_DIR) -list

## test-bash-helpers: Build helper programs needed by bash tests
# heredoc5.sub round-trips $(BUILD_DIR)/config.h (needs 4096 < size <
# 65536) and version.h (512 < size < 4096) through here-documents. They
# are bash build artifacts absent from the vendored source tree, so
# generate deterministic stubs of the right sizes. Some trimmed fixture
# copies also omit y.tab.c and examples/loadables/Makefile, which the
# heredoc and glob-bracket tests read as source/build artifacts.
test-bash-helpers:
	@cd $(BASH_TESTS_DIR) && \
		[ -f recho ] || cc -o recho ../support/recho.c 2>/dev/null; \
		[ -f zecho ] || cc -o zecho ../support/zecho.c 2>/dev/null; \
		[ -f xcase ] || cc -o xcase ../support/xcase.c 2>/dev/null; \
		[ -f ../config.h ] || for i in $$(seq 1 128); do \
			printf '/* stub config.h line %03d for heredoc5.sub */\n' $$i; \
		done > ../config.h; \
		[ -f ../version.h ] || for i in $$(seq 1 16); do \
			printf '/* stub version.h line %03d for heredoc5.sub */\n' $$i; \
		done > ../version.h; \
		[ -f ../y.tab.c ] || for i in $$(seq 1 2048); do \
			printf '/* stub y.tab.c line %04d for heredoc5.sub */\n' $$i; \
		done > ../y.tab.c; \
		if [ ! -f ../examples/loadables/Makefile ]; then \
			mkdir -p ../examples/loadables; \
			{ \
				echo 'CC = cc'; \
				echo 'SHOBJ_STATUS = supported'; \
				echo 'SHOBJ_CC = cc'; \
				echo 'SHOBJ_CFLAGS = -fPIC'; \
				echo 'SHOBJ_LD = cc'; \
				case "$$(uname -s)" in \
					Darwin) echo 'SHOBJ_LDFLAGS = -shared -undefined dynamic_lookup' ;; \
					*) echo 'SHOBJ_LDFLAGS = -shared' ;; \
				esac; \
				echo 'SHOBJ_XLDFLAGS ='; \
				echo 'SHOBJ_LIBS ='; \
			} > ../examples/loadables/Makefile; \
		fi; \
		true

## hooks: Install the committed git hooks (pre-push .sibling-pins drift gate)
hooks:
	git config core.hooksPath scripts/hooks
	@echo "hooks: core.hooksPath -> scripts/hooks (bypass a hook with 'git push --no-verify')"

## tidy: Run go mod tidy, gofmt, and go vet
tidy:
	go mod tidy
	gofmt -s -w .
	go vet ./...

## clean: Remove built binaries
clean:
	rm -rf $(BIN_DIR)

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'
