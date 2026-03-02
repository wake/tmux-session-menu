#!/usr/bin/env bash
# tsm-hook.sh — Claude Code hook script for tsm status detection (Layer 1)
#
# This script is called by Claude Code hooks. It reads hook event JSON from
# stdin, maps the event to a tsm status, and writes a status JSON file that
# tsm can read.
#
# Usage in Claude Code hooks config (~/.claude/settings.json):
#   "hooks": {
#     "UserPromptSubmit": [{ "type": "command", "command": "bash /path/to/tsm-hook.sh" }],
#     "Stop":             [{ "type": "command", "command": "bash /path/to/tsm-hook.sh" }],
#     ...
#   }

set -euo pipefail

# --- Configuration -----------------------------------------------------------
TSM_STATUS_DIR="${TSM_STATUS_DIR:-$HOME/.config/tsm/status}"

# --- Guard: must be inside a tmux session ------------------------------------
if [[ -z "${TMUX:-}" ]]; then
  exit 0
fi

SESSION_NAME="$(tmux display-message -p '#S' 2>/dev/null || true)"
if [[ -z "$SESSION_NAME" ]]; then
  exit 0
fi

# --- Read stdin and extract hook_event_name ----------------------------------
INPUT="$(cat)"

HOOK_EVENT="$(printf '%s' "$INPUT" | python3 -c '
import sys, json
try:
    data = json.load(sys.stdin)
    print(data.get("hook_event_name", ""))
except Exception:
    print("")
' 2>/dev/null || true)"

# --- Map event to status -----------------------------------------------------
case "$HOOK_EVENT" in
  UserPromptSubmit) STATUS="running"  ;;
  Stop)             STATUS="idle"     ;;
  SessionStart)     STATUS="running"  ;;
  SessionEnd)       STATUS="idle"     ;;
  PermissionRequest) STATUS="waiting" ;;
  Notification)     STATUS="waiting"  ;;
  "")               exit 0            ;;
  *)                STATUS="running"  ;;
esac

# --- Write status JSON -------------------------------------------------------
mkdir -p "$TSM_STATUS_DIR"

TIMESTAMP="$(date +%s)"

TMP_FILE="$(mktemp "$TSM_STATUS_DIR/.tsm-hook.XXXXXX")"
printf '{"status":"%s","timestamp":%s,"event":"%s"}\n' \
  "$STATUS" "$TIMESTAMP" "$HOOK_EVENT" > "$TMP_FILE"
mv "$TMP_FILE" "$TSM_STATUS_DIR/$SESSION_NAME"
