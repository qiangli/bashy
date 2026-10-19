#!/bin/sh
# java-runner — a fence runner for a language bashy has no fence for: Java.
# Register it once, use it from any script:
#
#   bashy commands register java-runner --set exec.0=$PWD/examples/runners/java-runner.sh --set effects.0=exec
#   ~~~java as j !java-runner ... ~~~      j.main(args...)
#
# A runner is called as `runner <verb> <file> [args…]`: `methods` answers the
# verb table (one JSON object per line: name, optional signature, optional
# effects), every other verb serves the fence. stdout is the value.

case "$1" in
methods)
	printf '%s\n' '{"name":"main","effects":["read","write","exec"]}'
	exit 0
	;;
main)
	;;
*)
	echo "java-runner: unknown verb $1" >&2
	exit 2
	;;
esac

file="$2"
shift 2

# Find public class name in FILE (first "public class NAME")
class_name=$(awk '/public[ \t]+class[ \t]+[A-Za-z0-9_$]+/ {
	for (i = 1; i <= NF; i++) {
		if ($i == "class" && (i + 1) <= NF) {
			sub(/[^A-Za-z0-9_$].*/, "", $(i+1))
			print $(i+1)
			exit
		}
	}
}' "$file")

if [ -z "$class_name" ]; then
	echo "java-runner: no public class found in $file" >&2
	exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd -P)
pin_file="${JAVA_RUNNER_PIN:-$script_dir/java-pin.tsv}"

javac=""
java=""

find_cached_jdk() {
	[ -d "$cache_dir" ] || return 1
	if [ -x "$cache_dir/bin/javac" ] && [ -x "$cache_dir/bin/java" ]; then
		javac="$cache_dir/bin/javac"
		java="$cache_dir/bin/java"
		return 0
	fi
	if [ -x "$cache_dir/Contents/Home/bin/javac" ] && [ -x "$cache_dir/Contents/Home/bin/java" ]; then
		javac="$cache_dir/Contents/Home/bin/javac"
		java="$cache_dir/Contents/Home/bin/java"
		return 0
	fi
	found=$(find "$cache_dir" -type f -name javac 2>/dev/null | head -n 1)
	if [ -n "$found" ] && [ -x "$found" ] && [ -x "${found%/*}/java" ]; then
		javac="$found"
		java="${found%/*}/java"
		return 0
	fi
	return 1
}

# JDK resolution
if [ -n "${JAVA_HOME:-}" ]; then
	javac="$JAVA_HOME/bin/javac"
	java="$JAVA_HOME/bin/java"
	if [ ! -x "$javac" ] || [ ! -x "$java" ]; then
		echo "java-runner: no JDK: set JAVA_HOME or JAVA_RUNNER_FETCH=1" >&2
		exit 1
	fi
else
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)
	case "$arch" in
		aarch64) arch=arm64 ;;
	esac

	[ -f "$pin_file" ] || { echo "java-runner: pin file not found: $pin_file" >&2; exit 1; }

	pin_version=""
	pin_url=""
	pin_sha256=""
	pin_license=""

	while IFS='	' read -r r_os r_arch r_version r_url r_sha256 r_license || [ -n "$r_os" ]; do
		case "$r_os" in
			\#*|"") continue ;;
		esac
		if [ "$r_os" = "$os" ] && [ "$r_arch" = "$arch" ]; then
			pin_version="$r_version"
			pin_url="$r_url"
			pin_sha256="$r_sha256"
			pin_license="$r_license"
			break
		fi
	done < "$pin_file"

	if [ -z "$pin_version" ]; then
		echo "java-runner: unsupported platform $os/$arch (not in pin file)" >&2
		exit 1
	fi

	bashy_home="${BASHY_HOME:-$HOME/.bashy}"
	cache_dir="$bashy_home/custom/java/$pin_version"

	if find_cached_jdk; then
		: # JDK found in cache
	elif [ "${JAVA_RUNNER_FETCH:-0}" = "1" ]; then
		dl_tmp=$(mktemp "${TMPDIR:-/tmp}/jdk-download.XXXXXX")
		if command -v curl >/dev/null 2>&1; then
			curl -sSfL "$pin_url" -o "$dl_tmp" || { rm -f "$dl_tmp"; echo "java-runner: download failed: $pin_url" >&2; exit 1; }
		elif command -v wget >/dev/null 2>&1; then
			wget -qO "$dl_tmp" "$pin_url" || { rm -f "$dl_tmp"; echo "java-runner: download failed: $pin_url" >&2; exit 1; }
		elif case "$pin_url" in file://*) true ;; *) false ;; esac; then
			cp "${pin_url#file://}" "$dl_tmp" || { rm -f "$dl_tmp"; echo "java-runner: copy failed: $pin_url" >&2; exit 1; }
		else
			rm -f "$dl_tmp"
			echo "java-runner: neither curl nor wget found" >&2
			exit 1
		fi

		actual_sha256=""
		if command -v sha256sum >/dev/null 2>&1; then
			actual_sha256=$(sha256sum "$dl_tmp" | awk '{print $1}')
		elif command -v shasum >/dev/null 2>&1; then
			actual_sha256=$(shasum -a 256 "$dl_tmp" | awk '{print $1}')
		else
			rm -f "$dl_tmp"
			echo "java-runner: neither sha256sum nor shasum found" >&2
			exit 1
		fi

		actual_sha256=$(echo "$actual_sha256" | tr '[:upper:]' '[:lower:]')
		pin_sha256=$(echo "$pin_sha256" | tr '[:upper:]' '[:lower:]')

		if [ "$actual_sha256" != "$pin_sha256" ]; then
			rm -f "$dl_tmp"
			echo "java-runner: checksum mismatch" >&2
			exit 1
		fi

		mkdir -p "$cache_dir"
		tar -xzf "$dl_tmp" -C "$cache_dir" || {
			rm -f "$dl_tmp"
			echo "java-runner: tar extraction failed" >&2
			exit 1
		}
		touch "$cache_dir/.marker"
		rm -f "$dl_tmp"

		if ! find_cached_jdk; then
			echo "java-runner: javac or java not found in extracted JDK" >&2
			exit 1
		fi
	else
		echo "java-runner: no JDK: set JAVA_HOME or JAVA_RUNNER_FETCH=1" >&2
		exit 1
	fi
fi

# Compile and run
case "$file" in
	*/*) dir="${file%/*}" ;;
	*)   dir="." ;;
esac

mkdir -p "$dir/src" "$dir/classes"
cp "$file" "$dir/src/$class_name.java" || exit 1

"$javac" -d "$dir/classes" "$dir/src/$class_name.java" || exit $?

if [ -n "${JAVA_RUNNER_CLASSPATH:-}" ]; then
	cp_arg="$dir/classes:$JAVA_RUNNER_CLASSPATH"
else
	cp_arg="$dir/classes"
fi

exec "$java" -cp "$cp_arg" "$class_name" "$@"
