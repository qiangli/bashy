#!/bin/sh
# Build and verify the one lean linux/amd64 Bashy artifact consumed by Cloudbox.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
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
smoke_runtime=
cleanup() {
	rm -f "$deps_tmp"
	if [ -n "$smoke_tmp" ] && [ -d "$smoke_tmp" ]; then
		rm -rf "$smoke_tmp"
	fi
	if [ -n "$smoke_runtime" ] && [ -n "$smoke_image" ]; then
		"$smoke_runtime" image rm -f "$smoke_image" >/dev/null 2>&1 || true
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
	echo "verify-bashy-scratch: PASS bashy --version and supervisord --help"
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
	command -v "$BASHY_SCRATCH_RUNTIME" >/dev/null 2>&1 || fail "requested runtime not found: $BASHY_SCRATCH_RUNTIME"
	if ! "$BASHY_SCRATCH_RUNTIME" info >/dev/null 2>&1; then
		fail "requested runtime is not usable: $BASHY_SCRATCH_RUNTIME"
	fi
	smoke_runtime=$BASHY_SCRATCH_RUNTIME
else
	for candidate in podman docker nerdctl; do
		if command -v "$candidate" >/dev/null 2>&1 && "$candidate" info >/dev/null 2>&1; then
			smoke_runtime=$candidate
			break
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
printf '%s\n' 'FROM scratch' 'COPY bashy /bashy' 'ENTRYPOINT ["/bashy"]' >"$smoke_tmp/Containerfile"
smoke_image="localhost/bashy-scratch-smoke:$$"
"$smoke_runtime" build --platform linux/amd64 --network none \
	-f "$smoke_tmp/Containerfile" -t "$smoke_image" "$smoke_tmp" >/dev/null ||
	fail "$smoke_runtime could not build the FROM scratch smoke image"
"$smoke_runtime" run --rm --platform linux/amd64 "$smoke_image" --version >/dev/null ||
	fail "bashy --version failed in the FROM scratch image"
"$smoke_runtime" run --rm --platform linux/amd64 "$smoke_image" supervisord --help >/dev/null ||
	fail "bashy supervisord --help failed in the FROM scratch image"
echo "verify-bashy-scratch: PASS real FROM scratch smoke via $smoke_runtime"
