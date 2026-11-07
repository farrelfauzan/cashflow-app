#!/usr/bin/env bash
# Fails when a change adds a file over 1 MB in any of its commits, or adds or modifies a binary
# file outside the allowlist. A build output committed by accident (a Go binary, an archive) is
# caught before review, even when a later commit removes it again.
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

# Size: every blob any commit of the change introduces, not only the final tree. A file added in
# one commit and removed in the next still lands in main's history when the PR is merged with a
# merge commit.
while read -r type size path; do
  [ "$type" = "blob" ] || continue
  if [ "$size" -gt "$max_bytes" ]; then
    echo "::error::${path:-a blob} is $size bytes in this change's history, over the 1 MB limit"
    failed=1
  fi
done < <(git rev-list --objects "$base..HEAD" |
  git cat-file --batch-check='%(objecttype) %(objectsize) %(rest)')

# Binary: files the change adds or modifies, in its final state. -z keeps paths unquoted (an
# accented name, a tab or a quote), and --numstat prints "-" as the line counts of a binary file.
while IFS=$'\t' read -r -d '' added _ path; do
  [ -n "$path" ] || continue
  if [ "$added" = "-" ] && ! allowed_binary "$path"; then
    echo "::error file=$path::$path is a binary file outside the allowlist in infra/ci/check-added-files.sh"
    failed=1
  fi
done < <(git diff -z --numstat --no-renames --diff-filter=AM "$base...HEAD")

if [ "$failed" -ne 0 ]; then
  echo "A file still in the change: remove it in a new commit (git rm --cached <path>) and ignore build outputs in .gitignore."
  echo "A large file only in an earlier commit stays in history: the PR must be squash-merged (or rebased to drop it, if the team agrees)."
  exit 1
fi
echo "No file over 1 MB in this change's history and no binary outside the allowlist."
