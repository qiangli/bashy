#!/bin/sh
# Build and verify the one lean linux/amd64 Bashy artifact consumed by Cloudbox.
set -eu

root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
artifact=${BASHY_SCRATCH_ARTIFACT:-bin/scratch/bashy-linux-amd64}
case "$artifact" in
	/*) ;;
	*) artifact=$root/$artifact ;;
esac
cd "$root"

fail() {
	echo "verify-bashy-scratch: FAIL: $*" >&2
	exit 1
}

# The verifier owns the build so a stale artifact can never produce a green
# result. The profile target is separate from normal lean and release builds.
make -C "$root" --no-print-directory build-bashy-scratch \
	BASHY_SCRATCH_ARTIFACT="$artifact"
[ -f "$artifact" ] || fail "build did not produce $artifact"
[ -x "$artifact" ] || fail "artifact is not executable: $artifact"

# debug/elf is the deterministic cross-host authority. It works on macOS and
# Windows hosts without a container daemon and rejects malformed/unreadable ELF.
go run "$root/tools/elfaudit" "$artifact" || fail "structural ELF audit failed"

# When native ELF tooling can parse the artifact, independently confirm the two
# dynamic-link fields. Tools that exist but do not understand ELF (notably some
# host objdump builds) are skipped rather than mistaken for a successful audit.
elf_tool=
elf_report=
for candidate in readelf llvm-readelf; do
	if command -v "$candidate" >/dev/null 2>&1; then
		if report=$($candidate -l -d "$artifact" 2>/dev/null); then
			elf_tool=$candidate
			elf_report=$report
			break
		fi
	fi
done
if [ -z "$elf_tool" ]; then
	for candidate in objdump llvm-objdump; do
		if command -v "$candidate" >/dev/null 2>&1; then
			if report=$($candidate -p "$artifact" 2>/dev/null) &&
				printf '%s\n' "$report" | grep -Eiq 'elf64|elf file'; then
				elf_tool=$candidate
				elf_report=$report
				break
			fi
		fi
	done
fi
if [ -n "$elf_tool" ]; then
	if printf '%s\n' "$elf_report" | grep -Eq '(^|[[:space:]])INTERP([[:space:]]|$)|\(NEEDED\)|(^|[[:space:]])NEEDED([[:space:]]|$)'; then
		fail "$elf_tool found an ELF INTERP or NEEDED entry"
	fi
	echo "verify-bashy-scratch: PASS $elf_tool confirms no INTERP or NEEDED entries"
else
	echo "verify-bashy-scratch: INFO no native ELF inspector parsed the artifact; debug/elf audit remains authoritative"
fi

# The tagged graph is part of the profile contract. CGO=0 by itself does not
# prevent purego from loading glibc dynamically at runtime.
deps_tmp=$(mktemp "${TMPDIR:-/tmp}/bashy-scratch-deps.XXXXXX")
smoke_tmp=
smoke_image=
smoke_volume=
smoke_runtime=
timeout_cmd=
if command -v timeout >/dev/null 2>&1; then
	timeout_cmd=timeout
elif command -v gtimeout >/dev/null 2>&1; then
	timeout_cmd=gtimeout
fi
runtime_command() {
	"$timeout_cmd" --kill-after=5s 90s "$smoke_runtime" "$@"
}
cleanup() {
	rm -f "$deps_tmp"
	if [ -n "$smoke_tmp" ] && [ -d "$smoke_tmp" ]; then
		rm -rf "$smoke_tmp"
	fi
	if [ -n "$smoke_runtime" ] && [ -n "$smoke_volume" ]; then
		runtime_command volume rm -f "$smoke_volume" >/dev/null 2>&1 || true
	fi
	if [ -n "$smoke_runtime" ] && [ -n "$smoke_image" ]; then
		runtime_command image rm -f "$smoke_image" >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT HUP INT TERM

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go list -deps -tags bashy_scratch \
	./cmd/bashy >"$deps_tmp" || fail "could not enumerate the scratch dependency graph"
if grep -Eq '(^|/)purego($|/)' "$deps_tmp"; then
	grep -E '(^|/)purego($|/)' "$deps_tmp" >&2
	fail "scratch dependency graph contains purego"
fi
echo "verify-bashy-scratch: PASS scratch dependency graph contains no purego"

run_smoke() {
	"$1" --version >/dev/null || fail "bashy --version failed from the scratch artifact"
	"$1" supervisord --help >/dev/null || fail "bashy supervisord --help failed from the scratch artifact"
	set +e
	"$1" supervisord DAG.md root >/dev/null 2>&1
	status=$?
	set -e
	[ "$status" = 2 ] || fail "bashy supervisord DAG.md TARGET was not refused (exit $status)"
	echo "verify-bashy-scratch: PASS bashy --version, supervisord --help and the retired-form refusal"
}

host_os=$(go env GOOS)
host_arch=$(go env GOARCH)
native_smoked=0
if [ "$host_os/$host_arch" = linux/amd64 ]; then
	run_smoke "$artifact"
	native_smoked=1
fi

# A usable runtime upgrades the command smoke to a literal FROM scratch proof.
# An installed but stopped daemon is not an available runtime; deterministic
# structural checks above still provide a useful gate on cross-build hosts.
if [ -n "${BASHY_SCRATCH_RUNTIME:-}" ]; then
	[ -n "$timeout_cmd" ] || fail "a timeout command is required for container smoke"
	command -v "$BASHY_SCRATCH_RUNTIME" >/dev/null 2>&1 || fail "requested runtime not found: $BASHY_SCRATCH_RUNTIME"
	smoke_runtime=$BASHY_SCRATCH_RUNTIME
	if ! runtime_command info >/dev/null 2>&1; then
		fail "requested runtime is not usable: $BASHY_SCRATCH_RUNTIME"
	fi
else
	for candidate in podman docker nerdctl; do
		if [ -n "$timeout_cmd" ] && command -v "$candidate" >/dev/null 2>&1; then
			smoke_runtime=$candidate
			if runtime_command info >/dev/null 2>&1; then
				break
			fi
			smoke_runtime=
		fi
	done
fi

if [ -z "$smoke_runtime" ]; then
	if [ "$native_smoked" = 1 ]; then
		echo "verify-bashy-scratch: SKIP FROM-scratch smoke (a usable container runtime is unavailable)"
	else
		echo "verify-bashy-scratch: SKIP command/FROM-scratch smoke (linux/amd64 execution and a usable container runtime are unavailable)"
	fi
	exit 0
fi

smoke_tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-scratch-smoke.XXXXXX")
cp "$artifact" "$smoke_tmp/bashy"
# The image is the Sprint 418 S19 shape: the entrypoint is the service monitor
# as PID 1, and the dag root it serves is a unit (exec: bashy dag DAG.md
# TARGET) shipped in the shared unit ring. Only /bashy is a payload; the unit
# and the one-target DAG are data.
printf '%s\n' '## Tasks' '' '### root' 'Root target of the smoke unit.' '' '```bash' \
	'echo scratch-root-ran' '```' >"$smoke_tmp/DAG.md"
printf '%s\n' 'name: root' 'kind: service' 'enabled: true' \
	'exec: [bashy, dag, /srv/DAG.md, root]' \
	'restart: {backoff: {min: 100ms, max: 1s}}' \
	'stop: {timeout: 2s}' >"$smoke_tmp/root.yaml"
printf '%s\n' 'FROM scratch' 'COPY bashy /bashy' \
	'COPY DAG.md /srv/DAG.md' 'COPY root.yaml /etc/bashy/services/root.yaml' \
	'ENV BASHY_SERVICES_PATH=/etc/bashy/services BASHY_HOME=/run/bashy' \
	'ENTRYPOINT ["/bashy", "supervisord", "--pid1"]' >"$smoke_tmp/Containerfile"
smoke_image="localhost/bashy-scratch-smoke:$$"
runtime_command build --platform linux/amd64 --network none \
	-f "$smoke_tmp/Containerfile" -t "$smoke_image" "$smoke_tmp" >"$smoke_tmp/build.out" 2>&1 ||
	fail "$smoke_runtime could not build the FROM scratch smoke image: $(cat "$smoke_tmp/build.out")"
runtime_command run --rm --platform linux/amd64 --entrypoint /bashy "$smoke_image" --version >/dev/null ||
	fail "bashy --version failed in the FROM scratch image"
runtime_command run --rm --platform linux/amd64 --entrypoint /bashy "$smoke_image" supervisord --help >/dev/null ||
	fail "bashy supervisord --help failed in the FROM scratch image"

# The entrypoint runs the selected root as PID 1. The embedded catalog also
# contains enabled long-running services, so --only root makes completion of
# this one-shot unit end the container.
# A unit's output goes to its service log under BASHY_HOME, so BASHY_HOME is a
# named volume and a second container reads the log back with `service logs`.
smoke_volume="bashy-scratch-smoke-$$"
runtime_command run --rm --platform linux/amd64 -v "$smoke_volume:/run/bashy" --tmpfs /tmp \
	"$smoke_image" --until-done --only root >"$smoke_tmp/run.out" 2>&1 ||
	fail "bashy supervisord --pid1 did not run the root unit to completion in the FROM scratch image: $(cat "$smoke_tmp/run.out")"
grep -q 'root: active -> inactive (completed (exit 0))' "$smoke_tmp/run.out" ||
	fail "the monitor did not report the root unit completed: $(cat "$smoke_tmp/run.out")"
unit_log=$(runtime_command run --rm --platform linux/amd64 -v "$smoke_volume:/run/bashy" \
	--entrypoint /bashy "$smoke_image" service logs root 2>&1) ||
	fail "bashy service logs root failed in the FROM scratch image: $unit_log"
printf '%s\n' "$unit_log" | grep -q 'scratch-root-ran' ||
	fail "the root unit's dag target did not run under the PID 1 monitor: $unit_log"
echo "verify-bashy-scratch: PASS supervisord --pid1 ran the root unit in the FROM scratch image"

# The old DAG.md TARGET form is refused (exit 2) with a pointer to the unit form.
set +e
refused=$(runtime_command run --rm --platform linux/amd64 --entrypoint /bashy "$smoke_image" \
	supervisord /srv/DAG.md root 2>&1)
refused_status=$?
set -e
[ "$refused_status" = 2 ] ||
	fail "supervisord DAG.md TARGET exited $refused_status in the FROM scratch image, want 2: $refused"
printf '%s\n' "$refused" | grep -q 'exec: \[bashy, dag, /srv/DAG.md, root\]' ||
	fail "the refusal does not point at the unit form: $refused"
echo "verify-bashy-scratch: PASS supervisord DAG.md TARGET refused with the unit form"
echo "verify-bashy-scratch: PASS real FROM scratch smoke via $smoke_runtime"
