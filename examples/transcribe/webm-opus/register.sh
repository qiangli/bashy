#!/bin/sh
set -eu
: "${BASHY_BIN:=bashy}"
if [ "$#" -ne 1 ]; then echo 'usage: register.sh ABSOLUTE-ADAPTER-PATH' >&2; exit 2; fi
case "$1" in /*|[A-Za-z]:/*|[A-Za-z]:\\*|\\\\*) ;; *) echo 'absolute executable path required' >&2; exit 2;; esac
test -f "$1" || { echo 'adapter executable missing' >&2; exit 2; }
"$BASHY_BIN" commands add transcribe-webm-opus \
 --set "exec.0=$1" \
 --set provides.0=transcribe.decode \
 --set accepts.0.path=/version --set 'accepts.0.pattern=^bashy-transcribe-stage-v1$' \
 --set accepts.1.path=/artifacts/0/path --set 'accepts.1.pattern=(?i)\.webm$' \
 --set accepts.2.path=/stage --set 'accepts.2.pattern=^decode$' \
 --set effects.0=read --set effects.1=write --set effects.2=destroy --set effects.3=exec \
 --set os.0=linux --set os.1=darwin --set os.2=windows --set requires=arch=amd64,arm64
