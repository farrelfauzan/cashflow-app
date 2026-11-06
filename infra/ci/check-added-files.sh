#!/usr/bin/env bash
# Fails when a change adds or modifies a file over 1 MB, or a binary file outside the allowlist.
# A build output committed by accident (a Go binary, an archive) is caught before review.
#
# Usage: infra/ci/check-added-files.sh <base ref>   (CI passes the PR's base, e.g. origin/main)
# Run it locally before pushing: infra/ci/check-added-files.sh origin/main
set -euo pipefail

base="${1:?usage: check-added-files.sh <base ref>}"
max_bytes=$((1024 * 1024))

# Binary files that belong in the repository. Add a pattern here, in the same PR, when a new kind
# is needed, so the reviewer sees it.
allowed_binary() {
  case "$1" in
    apps/web/public/*.png | apps/web/public/*.ico | apps/web/public/*.webp) return 0 ;;
    services/api/internal/qrimage/fonts/*.ttf) return 0 ;;
    services/api/internal/qrimage/testdata/*.golden.png) return 0 ;;
    *) return 1 ;;
  esac
}

failed=0
# --numstat prints "-<TAB>-<TAB>path" for a file git treats as binary.
while IFS=$'\t' read -r added _ path; do
  [ -n "$path" ] || continue
  size=$(git cat-file -s "HEAD:$path")
  if [ "$size" -gt "$max_bytes" ]; then
    echo "::error file=$path::$path is $size bytes, over the 1 MB limit"
    failed=1
  fi
  if [ "$added" = "-" ] && ! allowed_binary "$path"; then
    echo "::error file=$path::$path is a binary file outside the allowlist in infra/ci/check-added-files.sh"
    failed=1
  fi
done < <(git diff --numstat --no-renames --diff-filter=AM "$base...HEAD")

if [ "$failed" -ne 0 ]; then
  echo "Remove the file in a new commit (git rm --cached <path>) and ignore build outputs in .gitignore."
  exit 1
fi
echo "No file over 1 MB and no binary outside the allowlist."
