#!/bin/sh
# Normalize measured TSV notes without changing the status or row denominator.
# Makefile buildID is either an exact release tag or a short commit SHA.
# Keep the documented <sha> placeholder for either identity, scoped to --version.
sed -E '
/^shell	--version	/ s/\(v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?\)$/(<sha>)/
s/(amd64|arm64|x86_64|aarch64|x64)/<arch>/g
s/v?[0-9]+\.[0-9]+(\.[0-9]+)?(-[A-Za-z0-9.]+)?/<ver>/g
s/\([0-9a-f]{7,12}\)/(<sha>)/g
s/ dev( |$)/ <ver>\1/g
s/-dev([ )]|$)/-<ver>\1/g
'
