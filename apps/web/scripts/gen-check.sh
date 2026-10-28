#!/bin/sh
# Fails when src/lib/api/generated is not exactly what `pnpm gen` produces from
# api/openapi.yaml. It regenerates in place and compares with a copy of the files that were
# there before, so new, changed and deleted files all count, and it works the same before the
# output is committed and in CI. On failure the fresh output is left in place: review it and
# commit it.
set -eu

dir=src/lib/api/generated
snapshot=$(mktemp -d)
trap 'rm -rf "$snapshot"' EXIT

if [ -d "$dir" ]; then
  cp -R "$dir/." "$snapshot/"
fi

pnpm gen

if ! diff -r "$snapshot" "$dir"; then
  echo "$dir is stale: run 'pnpm gen' (make web-gen) and commit the result." >&2
  exit 1
fi
echo "$dir is current."
