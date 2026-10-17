#!/usr/bin/env bash
# Hermetic unit test for scripts/genie-preview-workflow.sh summary counter,
# GENIE_REQUEST_TIMEOUT_MS export, and self-test (Sprint 455 story #2310 / R7).
#
# No models, no door daemon, no network. Uses a stub bashy to verify
# that the workflow script's summary correctly counts legs and that
# GENIE_REQUEST_TIMEOUT_MS is exported following turn timeout with caller precedence.
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
script="$here/genie-preview-workflow.sh"

pass=0
fail=0
ok() { pass=$((pass + 1)); echo "ok: $1"; }
bad() { fail=$((fail + 1)); echo "FAIL: $1" >&2; }

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/genie-test.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT

# Create a mock bashy executable that simulates local and cloud model turns
mock_bashy="$tmpdir/mock-bashy"
cat >"$mock_bashy" <<'EOF'
#!/usr/bin/env bash
door_file="${TMPDIR:-/tmp}/mock_door.$$"
case "${1:-}" in
	--version)
		echo "bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy"
		exit 0
		;;
	fetch)
		# health check: report alive if door is running
		[ -f "$door_file" ] && exit 0
		exit 1
		;;
	timeout)
		shift 2
		exec "$@"
		;;
	llm)
		case "${2:-}" in
			up)
				touch "$door_file"
				exit 0
				;;
			down)
				rm -f "$door_file"
				exit 0
				;;
		esac
		;;
	ollama)
		case "${2:-}" in
			list)
				echo "NAME ID SIZE MODIFIED"
				exit 0
				;;
			show)
				exit 0
				;;
			pull)
				exit 0
				;;
			rm)
				exit 0
				;;
		esac
		;;
	model)
		case "${2:-}" in
			list)
				if [ "${MOCK_CLOUD_AVAILABLE:-0}" = "1" ]; then
					echo "door-mock-model subscription api"
				fi
				exit 0
				;;
			show)
				exit 0
				;;
		esac
		;;
	secret)
		exit 0
		;;
	genie)
		if [ "${MOCK_GENIE_FAIL:-0}" = "1" ]; then
			exit 1
		fi
		if [ -n "${MOCK_RECORD_TIMEOUT_FILE:-}" ]; then
			printf '%s' "${GENIE_REQUEST_TIMEOUT_MS:-unset}" > "$MOCK_RECORD_TIMEOUT_FILE"
		fi
		# create expected file with token and output token
		printf 'genie-preview-ok\n' > hello.txt
		printf 'genie-preview-ok\n'
		exit 0
		;;
	*)
		exit 0
		;;
esac
exit 0
EOF
chmod +x "$mock_bashy"

# Case 1: macOS-C2 regression: passing local leg (--legs local)
# Must report 1 pass, 0 fail, 0 cloud skipped (not the hardcoded 0 pass, 1 fail, 3 cloud skipped)
out=$(TMPDIR="$tmpdir" BASHY="$mock_bashy" "$script" --legs local --isolate 2>&1) || true
line=$(printf '%s\n' "$out" | grep 'result: local=' || true)
expected_c2='result: local=0 cloud=-1 exit=0 (1 pass, 0 fail, 0 cloud skipped)'
case "$line" in
	*"$expected_c2"*)
		ok "local passing leg summary counts (1 pass, 0 fail, 0 cloud skipped)"
		;;
	*)
		bad "local passing leg summary: expected '$expected_c2', got '$line'"
		;;
esac

# Case 2: failing local leg (--legs local)
out_fail=$(TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_GENIE_FAIL=1 "$script" --legs local --isolate 2>&1) || true
line_fail=$(printf '%s\n' "$out_fail" | grep 'result: local=' || true)
expected_fail='result: local=1 cloud=-1 exit=1 (0 pass, 1 fail, 0 cloud skipped)'
case "$line_fail" in
	*"$expected_fail"*)
		ok "local failing leg summary counts (0 pass, 1 fail, 0 cloud skipped)"
		;;
	*)
		bad "local failing leg summary: expected '$expected_fail', got '$line_fail'"
		;;
esac

# Case 3: local passes, cloud skipped (--legs local,cloud without cloud models)
out_skip=$(TMPDIR="$tmpdir" BASHY="$mock_bashy" "$script" --legs local,cloud --isolate 2>&1) || true
line_skip=$(printf '%s\n' "$out_skip" | grep 'result: local=' || true)
expected_skip='result: local=0 cloud=3 exit=3 (1 pass, 0 fail, 1 cloud skipped)'
case "$line_skip" in
	*"$expected_skip"*)
		ok "local pass + cloud skipped summary counts (1 pass, 0 fail, 1 cloud skipped)"
		;;
	*)
		bad "local pass + cloud skipped summary: expected '$expected_skip', got '$line_skip'"
		;;
esac

# Case 4: local passes, cloud passes (--legs local,cloud with cloud model)
out_both=$(TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_CLOUD_AVAILABLE=1 "$script" --legs local,cloud --isolate 2>&1) || true
line_both=$(printf '%s\n' "$out_both" | grep 'result: local=' || true)
expected_both='result: local=0 cloud=0 exit=0 (2 pass, 0 fail, 0 cloud skipped)'
case "$line_both" in
	*"$expected_both"*)
		ok "both legs pass summary counts (2 pass, 0 fail, 0 cloud skipped)"
		;;
	*)
		bad "both legs pass summary: expected '$expected_both', got '$line_both'"
		;;
esac

# Case 5: --self-test flag on the script itself
if "$script" --self-test >"$tmpdir/selftest.out" 2>&1; then
	ok "genie-preview-workflow.sh --self-test passes"
else
	bad "genie-preview-workflow.sh --self-test failed or unsupported"
fi

# Case 6: GENIE_REQUEST_TIMEOUT_MS follows GENIE_PREVIEW_TURN_TIMEOUT (1500s -> 1500000ms)
rec_custom="$tmpdir/timeout_custom.out"
TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_RECORD_TIMEOUT_FILE="$rec_custom" GENIE_PREVIEW_TURN_TIMEOUT=1500 "$script" --legs local --isolate >/dev/null 2>&1 || true
got_custom=$(cat "$rec_custom" 2>/dev/null || echo "not-recorded")
if [ "$got_custom" = "1500000" ]; then
	ok "GENIE_REQUEST_TIMEOUT_MS follows GENIE_PREVIEW_TURN_TIMEOUT (1500s -> 1500000ms)"
else
	bad "GENIE_REQUEST_TIMEOUT_MS with TURN_TIMEOUT=1500: expected 1500000, got '$got_custom'"
fi

# Case 7: caller-set GENIE_REQUEST_TIMEOUT_MS is preserved (caller wins)
rec_caller="$tmpdir/timeout_caller.out"
TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_RECORD_TIMEOUT_FILE="$rec_caller" GENIE_REQUEST_TIMEOUT_MS=999999 GENIE_PREVIEW_TURN_TIMEOUT=1500 "$script" --legs local --isolate >/dev/null 2>&1 || true
got_caller=$(cat "$rec_caller" 2>/dev/null || echo "not-recorded")
if [ "$got_caller" = "999999" ]; then
	ok "caller-provided GENIE_REQUEST_TIMEOUT_MS is kept (caller wins)"
else
	bad "caller GENIE_REQUEST_TIMEOUT_MS: expected 999999, got '$got_caller'"
fi

# Case 8: default GENIE_REQUEST_TIMEOUT_MS follows default TURN_TIMEOUT (600s -> 600000ms)
rec_default="$tmpdir/timeout_default.out"
TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_RECORD_TIMEOUT_FILE="$rec_default" "$script" --legs local --isolate >/dev/null 2>&1 || true
got_default=$(cat "$rec_default" 2>/dev/null || echo "not-recorded")
if [ "$got_default" = "600000" ]; then
	ok "default GENIE_REQUEST_TIMEOUT_MS follows default TURN_TIMEOUT (600s -> 600000ms)"
else
	bad "default GENIE_REQUEST_TIMEOUT_MS: expected 600000, got '$got_default'"
fi

# Case 9: cloud leg exports GENIE_REQUEST_TIMEOUT_MS following TURN_TIMEOUT
rec_cloud="$tmpdir/timeout_cloud.out"
TMPDIR="$tmpdir" BASHY="$mock_bashy" MOCK_CLOUD_AVAILABLE=1 MOCK_RECORD_TIMEOUT_FILE="$rec_cloud" GENIE_PREVIEW_TURN_TIMEOUT=1200 "$script" --legs cloud --isolate >/dev/null 2>&1 || true
got_cloud=$(cat "$rec_cloud" 2>/dev/null || echo "not-recorded")
if [ "$got_cloud" = "1200000" ]; then
	ok "cloud leg exports GENIE_REQUEST_TIMEOUT_MS following TURN_TIMEOUT"
else
	bad "cloud leg GENIE_REQUEST_TIMEOUT_MS: expected 1200000, got '$got_cloud'"
fi

if [ "$fail" -gt 0 ]; then
	echo "test-genie-preview-summary: $pass passed, $fail FAILED" >&2
	exit 1
fi
echo "test-genie-preview-summary: all $pass cases passed"
exit 0
