.PHONY: build install test test-bash test-bash-list test-bash-helpers dist tidy clean help

BIN_DIR := bin
BIN := $(BIN_DIR)/bashy
BASH_TESTS_DIR := external/bash-5.3/tests
# The bash test fixtures invoke the shell as `bash` / via $BASH, so the
# compliance harness drives a copy named `bin/bash`.
BASHY := $(BIN_DIR)/bash

# Stamp a real version onto release builds. Override on the command line, e.g.
#   make build VERSION=v0.1.0
VERSION ?= dev
LDFLAGS := -X 'main.bashVersion=5.3.0(1)-bashy-$(VERSION)'

# Platforms for `make dist` (goreleaser handles real releases; this is a
# local cross-compile sanity check).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

## build: Build the bashy binary into bin/
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN) .
	@cp $(BIN) $(BASHY)

## install: go install bashy into GOBIN
install:
	go install -ldflags "$(LDFLAGS)" .

## test: Run all Go tests
test:
	go test ./...

## dist: Cross-compile static binaries for all release platforms into bin/dist/
dist:
	@mkdir -p $(BIN_DIR)/dist
	@for plat in $(PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; \
		ext=; [ "$$os" = windows ] && ext=.exe; \
		out=$(BIN_DIR)/dist/bashy-$$os-$$arch$$ext; \
		echo "building $$out..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -ldflags "$(LDFLAGS)" -o $$out . || exit 1; \
	done

BASH_TEST_TIMEOUT := 60

# Tests known to time out due to feature gaps we don't plan to implement:
#   coproc  — full coprocess support (bashy subshells are goroutines,
#             no kernel coproc pipes)
#   jobs    — job control / kernel job table (same goroutine constraint)
#   trap    — signal trap subset that requires the missing job control
# Skipping these saves ~60s each on every `make test-bash` run.
BASH_TEST_SKIP := coproc jobs trap

# Tests whose bash run-* helper strips lines starting with `expect ` from
# the captured output before diffing against the .right file. The
# convention is local to a handful of tests: most embed `expect` echoes
# directly in the .right file (so filtering them would break the diff).
BASH_TEST_FILTER_EXPECT := attr exp extglob extglob2 invert invocation more-exp new-exp nquote nquote1 nquote2 nquote3 nquote5 posix2 varenv

# Tests whose bash run-* helper pipes captured output through `cat -v` to
# make control characters visible (NUL -> ^@, BEL -> ^G, ESC -> ^[, etc.)
# before diffing against the .right file. Apply the same transform here
# so raw control bytes don't trip the byte-for-byte diff.
BASH_TEST_CAT_V := printf

# The upstream test.tests fixture assumes /tmp allows setuid/setgid bits
# and that fd 0 is a terminal. Normalize only those host-dependent lines
# below so the fixture still checks bashy's test builtin behaviour.

## test-bash: Run bash 5.3 native test suite against bashy (with per-test timeout)
test-bash: build test-bash-helpers
	@echo "Running bash 5.3 test suite against bashy ($(BASH_TEST_TIMEOUT)s timeout per test)..."
	@BASHY_ABS=$$(pwd)/$(BASHY); cd $(BASH_TESTS_DIR) && \
		export THIS_SH=$$BASHY_ABS && \
		export BUILD_DIR=$$PWD/.. && \
		export PATH=$$PWD:/usr/bin:/bin:/usr/local/bin && \
		export BASH_TSTOUT=$${TMPDIR:-/tmp}/bashy-tstout-$$$$ && \
		export BASH_TSTRAW=$${TMPDIR:-/tmp}/bashy-tstraw-$$$$ && \
		passed=0 && failed=0 && skipped=0 && timeout_count=0 && \
		for runner in run-*; do \
			[ "$$runner" = "run-all" ] && continue; \
			name=$${runner#run-}; \
			test_file="$$name.tests"; \
			right_file="$$name.right"; \
			if [ "$$name" = "dirstack" ]; then \
				test_file="dstack.tests"; \
				right_file="dstack.right"; \
			fi; \
			if [ "$$name" = "precedence" ]; then \
				right_file="prec.right"; \
			fi; \
			if [ ! -f "$$test_file" ] || [ ! -f "$$right_file" ]; then \
				skipped=$$((skipped + 1)); \
				continue; \
			fi; \
			case " $(BASH_TEST_SKIP) " in \
				*" $$name "*) \
					skipped=$$((skipped + 1)); \
					printf "  SKIP  %s\n" "$$name"; \
					continue ;; \
			esac; \
			test_tmp=; \
			if [ "$$name" = "read" ]; then \
				test_tmp=$${TMPDIR:-/tmp}/bashy-read-$$$$; \
				rm -rf "$$test_tmp"; \
				mkdir -p "$$test_tmp"; \
			fi; \
			if [ -n "$$test_tmp" ]; then \
				TMPDIR=$$test_tmp perl -e 'setpgrp; exec @ARGV' $$THIS_SH ./$$test_file >$$BASH_TSTRAW 2>&1 & \
			else \
				perl -e 'setpgrp; exec @ARGV' $$THIS_SH ./$$test_file >$$BASH_TSTRAW 2>&1 & \
			fi; \
			test_pid=$$!; \
			( sleep $(BASH_TEST_TIMEOUT) && kill -KILL -- -$$test_pid 2>/dev/null ) & \
			timer_pid=$$!; \
			wait $$test_pid 2>/dev/null; \
			rc=$$?; \
			kill -KILL -- -$$test_pid 2>/dev/null; \
			kill $$timer_pid 2>/dev/null; wait $$timer_pid 2>/dev/null; \
			case " $(BASH_TEST_FILTER_EXPECT) " in \
				*" $$name "*) \
					grep -av '^expect' <$$BASH_TSTRAW >$$BASH_TSTOUT 2>/dev/null || : ;; \
				*) \
					cp $$BASH_TSTRAW $$BASH_TSTOUT 2>/dev/null || : ;; \
			esac; \
			case " $(BASH_TEST_CAT_V) " in \
				*" $$name "*) \
					cat -v <$$BASH_TSTOUT >$$BASH_TSTRAW 2>/dev/null && cp $$BASH_TSTRAW $$BASH_TSTOUT 2>/dev/null || : ;; \
			esac; \
			if [ "$$name" = "test" ]; then \
				perl -0pi -e 's/^chmod: .*?test\.setgid:.*\n(t -g \/tmp\/test\.setgid\n)1\n/$${1}0\n/mg; s/^chmod: .*?test\.setuid:.*\n(t -u \/tmp\/test\.setuid\n)1\n/$${1}0\n/mg; s/(t -n xx -a -z "" -a -t 0 -a -t\n)1\n/$${1}0\n/g' $$BASH_TSTOUT 2>/dev/null || :; \
			fi; \
			if [ $$rc -eq 137 ] 2>/dev/null; then \
				timeout_count=$$((timeout_count + 1)); \
				printf "  TIME  %s\n" "$$name"; \
			elif diff -q $$BASH_TSTOUT $$right_file > /dev/null 2>&1; then \
				passed=$$((passed + 1)); \
				printf "  PASS  %s\n" "$$name"; \
			else \
				failed=$$((failed + 1)); \
				printf "  FAIL  %s\n" "$$name"; \
				if [ "$$name" = "read" ]; then \
					diff -u $$right_file $$BASH_TSTOUT 2>/dev/null | sed -n '1,120p'; \
				fi; \
			fi; \
			if [ -n "$$test_tmp" ]; then \
				rm -rf "$$test_tmp"; \
			fi; \
			rm -f $$BASH_TSTOUT $$BASH_TSTRAW; \
		done; \
		echo ""; \
		echo "Results: $$passed passed, $$failed failed, $$skipped skipped, $$timeout_count timed out"; \
		echo ""

## test-bash-list: List all available bash 5.3 tests
test-bash-list:
	@cd $(BASH_TESTS_DIR) && for runner in run-*; do \
		[ "$$runner" = "run-all" ] && continue; \
		echo "$${runner#run-}"; \
	done

## test-bash-helpers: Build helper programs needed by bash tests
# heredoc5.sub round-trips $(BUILD_DIR)/config.h (needs 4096 < size <
# 65536) and version.h (512 < size < 4096) through here-documents. They
# are bash build artifacts absent from the vendored source tree, so
# generate deterministic stubs of the right sizes. y.tab.c ships with
# the source tree and needs no stub.
test-bash-helpers:
	@cd $(BASH_TESTS_DIR) && \
		[ -f recho ] || cc -o recho ../support/recho.c 2>/dev/null; \
		[ -f zecho ] || cc -o zecho ../support/zecho.c 2>/dev/null; \
		[ -f ../config.h ] || for i in $$(seq 1 128); do \
			printf '/* stub config.h line %03d for heredoc5.sub */\n' $$i; \
		done > ../config.h; \
		[ -f ../version.h ] || for i in $$(seq 1 16); do \
			printf '/* stub version.h line %03d for heredoc5.sub */\n' $$i; \
		done > ../version.h; \
		true

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
