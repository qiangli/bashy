#!/usr/bin/env bash
# Provision the seven GNU Bash 5.3 corpus locales in Ubuntu/glibc under WSL and
# build the native PE bridge consumed through BASHY_HOST_LOCALE.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() {
  echo "windows-wsl-locales: ERROR: $*" >&2
  exit 2
}

case "$(go env GOOS)" in
windows) ;;
*) fail "this provisioner must run from native Windows Go (GOOS=$(go env GOOS))" ;;
esac
command -v wsl.exe >/dev/null 2>&1 || fail "WSL is unavailable; install/enable WSL 2, then rerun"
command -v cygpath >/dev/null 2>&1 || fail "cygpath is required (run this script from Git Bash)"

list_distros() {
  wsl.exe -l -q 2>/dev/null | tr -d '\000\r' | sed 's/^[^[:alnum:]]*//'
}

if ! list_distros | grep -qi '^Ubuntu$'; then
  echo "windows-wsl-locales: installing official Ubuntu distribution"
  wsl.exe --install --distribution Ubuntu --no-launch --web-download ||
    fail "wsl.exe could not install Ubuntu (a reboot or administrator setup may be required)"
fi
list_distros | grep -qi '^Ubuntu$' ||
  fail "Ubuntu installation returned successfully but no Ubuntu distribution is registered"

echo "windows-wsl-locales: provisioning the corpus locale set in Ubuntu"
wsl.exe -d Ubuntu -u root -- sh -c '
  set -eu
  export DEBIAN_FRONTEND=noninteractive
  apt-get -qq update
  apt-get -qq install -y locales
  grep -Fqx "en_US.UTF-8 UTF-8" /etc/locale.gen || printf "%s\n" "en_US.UTF-8 UTF-8" >> /etc/locale.gen
  grep -Fqx "de_DE.UTF-8 UTF-8" /etc/locale.gen || printf "%s\n" "de_DE.UTF-8 UTF-8" >> /etc/locale.gen
  grep -Fqx "fr_FR.ISO-8859-1 ISO-8859-1" /etc/locale.gen || printf "%s\n" "fr_FR.ISO-8859-1 ISO-8859-1" >> /etc/locale.gen
  grep -Fqx "ru_RU.CP1251 CP1251" /etc/locale.gen || printf "%s\n" "ru_RU.CP1251 CP1251" >> /etc/locale.gen
  grep -Fqx "zh_TW BIG5" /etc/locale.gen || printf "%s\n" "zh_TW BIG5" >> /etc/locale.gen
  grep -Fqx "zh_HK BIG5-HKSCS" /etc/locale.gen || printf "%s\n" "zh_HK BIG5-HKSCS" >> /etc/locale.gen
  locale-gen
  # Shift-JIS is not ASCII-compatible. Suppress only localedefs expected ASCII
  # warning, exactly as the Linux conformance job does.
  localedef --no-warnings=ascii -i ja_JP -f SHIFT_JIS ja_JP.SJIS
'

mkdir -p bin
wrapper_posix="$(pwd)/bin/windows-wsl-locale.exe"
CGO_ENABLED=0 go build -o "$wrapper_posix" ./tools/windowswslocale
wrapper_windows=$(cygpath -aw "$wrapper_posix")
export BASHY_WSL_DISTRO=Ubuntu
export BASHY_HOST_LOCALE="$wrapper_windows"
export BASHY_HOST_LOCALE_NAMES='en_US.UTF-8;zh_TW.big5;ja_JP.SJIS;fr_FR.ISO8859-1;de_DE.UTF-8;zh_HK.big5hkscs;ru_RU.CP1251'

normalize_charmap() {
  printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr -d '._\r\n-'
}

locales=(
  en_US.UTF-8 zh_TW.big5 ja_JP.SJIS fr_FR.ISO8859-1 de_DE.UTF-8
  zh_HK.big5hkscs ru_RU.CP1251
)
charmaps=(UTF-8 BIG5 SHIFT_JIS ISO-8859-1 UTF-8 BIG5-HKSCS CP1251)
categories=(LC_CTYPE LC_NUMERIC LC_TIME LC_COLLATE LC_MONETARY LC_MESSAGES)

"$wrapper_posix" -a >/dev/null || fail "native provider could not run locale -a"
for i in "${!locales[@]}"; do
  requested=${locales[$i]}
  expected=${charmaps[$i]}
  selected=$(LC_ALL="$requested" "$wrapper_posix") ||
    fail "$requested could not be selected"
  printf '%s\n' "$selected" | grep -Fxq "LC_CTYPE=\"$requested\"" ||
    fail "$requested silently selected a different LC_CTYPE"
  actual=$(LC_ALL="$requested" "$wrapper_posix" charmap) ||
    fail "$requested could not answer locale charmap"
  if [ "$(normalize_charmap "$actual")" != "$(normalize_charmap "$expected")" ]; then
    fail "$requested returned charmap '$actual', expected '$expected'"
  fi
  for category in "${categories[@]}"; do
    category_output=$(LC_ALL="$requested" "$wrapper_posix" -k "$category") ||
      fail "$requested could not answer locale -k $category"
    if [ "$category" = LC_CTYPE ]; then
      category_charmap=$(printf '%s\n' "$category_output" | sed -n 's/^charmap="\(.*\)"$/\1/p')
      if [ "$(normalize_charmap "$category_charmap")" != "$(normalize_charmap "$expected")" ]; then
        fail "$requested LC_CTYPE returned charmap '$category_charmap', expected '$expected'"
      fi
    fi
  done
done

# A normal Windows invocation can source this file before running the fixture
# harness. GitHub Actions also receives the values in each subsequent step.
env_file="$(pwd)/bin/windows-wsl-locales.env"
{
  printf 'export BASHY_WSL_DISTRO=%q\n' "$BASHY_WSL_DISTRO"
  printf 'export BASHY_HOST_LOCALE=%q\n' "$BASHY_HOST_LOCALE"
  printf 'export BASHY_HOST_LOCALE_NAMES=%q\n' "$BASHY_HOST_LOCALE_NAMES"
} > "$env_file"
if [ -n "${GITHUB_ENV:-}" ]; then
  {
    printf 'BASHY_WSL_DISTRO=%s\n' "$BASHY_WSL_DISTRO"
    printf 'BASHY_HOST_LOCALE=%s\n' "$BASHY_HOST_LOCALE"
    printf 'BASHY_HOST_LOCALE_NAMES=%s\n' "$BASHY_HOST_LOCALE_NAMES"
  } >> "$GITHUB_ENV"
fi

echo "windows-wsl-locales: ready: seven glibc locales via $BASHY_HOST_LOCALE"
echo "windows-wsl-locales: environment: $env_file"
