#!/usr/bin/env bash
# Stop the team. Messages, status and digests in team/state/ are kept.
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

if tmux has-session -t "$TEAM_SESSION" 2>/dev/null; then
  tmux kill-session -t "$TEAM_SESSION"
  rm -f "$STATE_DIR"/panes/*
  echo "stopped '$TEAM_SESSION'"
else
  echo "'$TEAM_SESSION' is not running"
fi
