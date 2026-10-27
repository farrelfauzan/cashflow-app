#!/usr/bin/env bash
# Start the four-agent team in tmux and attach to it.
#
#   team/start.sh            start (or re-attach if it is already running)
#   team/start.sh --detach   start without attaching
#
# Layout: window "team" has the orchestrator on the left (talk to it here) and backend,
# frontend and reviewer stacked on the right. Window "clock" runs the hourly digest timer.
set -euo pipefail
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/common.sh"

command -v tmux >/dev/null || die "tmux is not installed"
[[ -x "$CLAUDE_BIN" ]] || command -v "$CLAUDE_BIN" >/dev/null || die "claude not found at $CLAUDE_BIN (set CLAUDE_BIN)"

attach=1
[[ "${1:-}" == "--detach" ]] && attach=0

attach_session() {
  [[ $attach -eq 1 ]] || return 0
  if [[ -n "${TMUX:-}" ]]; then
    tmux switch-client -t "$TEAM_SESSION"
  else
    exec tmux attach -t "$TEAM_SESSION"
  fi
}

if tmux has-session -t "$TEAM_SESSION" 2>/dev/null; then
  echo "team already running in tmux session '$TEAM_SESSION'"
  attach_session
  exit 0
fi

init_state
if [[ ! -f "$STATE_DIR/STATUS.md" ]]; then
  cat >"$STATE_DIR/STATUS.md" <<'EOF'
# Team status

Maintained by the orchestrator. Newest information first inside each section.

## Needs the user

(nothing)

## In progress

(nothing)

## Waiting for review

(nothing)

## Done

(nothing)
EOF
fi

launch() { printf '%q %q' "$TEAM_DIR/bin/team-launch" "$1"; }

new_pane() { # new_pane <agent> <tmux split args...>
  local agent="$1"
  shift
  local id
  id="$(tmux "$@" -P -F '#{pane_id}' "$(launch "$agent")")"
  echo "$id" >"$STATE_DIR/panes/$agent"
  tmux set-option -p -t "$id" @agent "$agent"
}

# -x/-y matter only for a detached start; tmux resizes to the client on attach.
orch_id="$(tmux new-session -d -s "$TEAM_SESSION" -n team -c "$REPO_DIR" -x 240 -y 64 \
  -P -F '#{pane_id}' "$(launch orchestrator)")"
echo "$orch_id" >"$STATE_DIR/panes/orchestrator"
tmux set-option -p -t "$orch_id" @agent orchestrator

new_pane backend split-window -h -t "$orch_id" -c "$REPO_DIR"
new_pane frontend split-window -v -t "$(cat "$STATE_DIR/panes/backend")" -c "$REPO_DIR"
new_pane reviewer split-window -v -t "$(cat "$STATE_DIR/panes/frontend")" -c "$REPO_DIR"

tmux select-layout -t "$TEAM_SESSION:team" main-vertical
tmux resize-pane -t "$orch_id" -x '45%'

tmux set-option -t "$TEAM_SESSION" mouse on
tmux set-option -t "$TEAM_SESSION" remain-on-exit on
tmux set-option -t "$TEAM_SESSION" pane-border-status top
tmux set-option -t "$TEAM_SESSION" pane-border-format ' #{@agent} '
tmux select-pane -t "$orch_id"

tmux new-window -d -t "$TEAM_SESSION" -n clock -c "$REPO_DIR" "$TEAM_DIR/bin/team-clock"

echo "team started in tmux session '$TEAM_SESSION' (detach: Ctrl-b d, stop: team/stop.sh)"
attach_session
