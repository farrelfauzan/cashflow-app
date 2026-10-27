# Shared by every script in team/bin. Source it, do not run it.

TEAM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_DIR="$(cd "$TEAM_DIR/.." && pwd)"
STATE_DIR="$TEAM_DIR/state"
AGENTS="orchestrator backend frontend reviewer"

# shellcheck source=../config.env
source "$TEAM_DIR/config.env"

is_agent() {
  case " $AGENTS " in
    *" $1 "*) return 0 ;;
  esac
  return 1
}

die() {
  echo "team: $*" >&2
  exit 1
}

warn() {
  echo "team: $*" >&2
}

init_state() {
  local a
  mkdir -p "$STATE_DIR"/{panes,locks,tasks,digests,reviews,worktrees,prompts}
  for a in $AGENTS; do
    mkdir -p "$STATE_DIR/inbox/$a/read"
  done
  touch "$STATE_DIR/messages.log"
}
