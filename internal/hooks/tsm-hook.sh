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

# --- Read stdin and extract fields -------------------------------------------
INPUT="$(cat)"

eval "$(printf '%s' "$INPUT" | python3 -c '
import sys, json, shlex
try:
    data = json.load(sys.stdin)
    print("HOOK_EVENT=" + shlex.quote(data.get("hook_event_name", "")))
    print("NOTIF_TYPE=" + shlex.quote(data.get("notification_type", "")))
except Exception:
    print("HOOK_EVENT=")
    print("NOTIF_TYPE=")
' 2>/dev/null || { echo 'HOOK_EVENT='; echo 'NOTIF_TYPE='; })"

# --- Map event to status + ai_type ----------------------------------------
case "$HOOK_EVENT" in
  SessionEnd)        STATUS="idle";    AI_TYPE="" ;;
  UserPromptSubmit)  STATUS="running"; AI_TYPE="claude" ;;
  Stop)              STATUS="idle";    AI_TYPE="claude" ;;
  SessionStart)      STATUS="running"; AI_TYPE="claude" ;;
  PermissionRequest) STATUS="waiting"; AI_TYPE="claude" ;;
  Notification)
    case "$NOTIF_TYPE" in
      permission_prompt)    STATUS="waiting"; AI_TYPE="claude" ;;
      elicitation_dialog)   STATUS="waiting"; AI_TYPE="claude" ;;
      idle_prompt)          STATUS="idle";    AI_TYPE="claude" ;;
      auth_success)         STATUS="running"; AI_TYPE="claude" ;;
      *)                    STATUS="waiting"; AI_TYPE="claude" ;;
    esac
    ;;
  "")                exit 0            ;;
  *)                 STATUS="running"; AI_TYPE="claude" ;;
esac

# --- Write status JSON -------------------------------------------------------
mkdir -p "$TSM_STATUS_DIR"

TIMESTAMP="$(date +%s)"

TMP_FILE="$(mktemp "$TSM_STATUS_DIR/.tsm-hook.XXXXXX")"
printf '{"status":"%s","timestamp":%s,"event":"%s","ai_type":"%s","notification_type":"%s"}\n' \
  "$STATUS" "$TIMESTAMP" "$HOOK_EVENT" "$AI_TYPE" "$NOTIF_TYPE" > "$TMP_FILE"
mv "$TMP_FILE" "$TSM_STATUS_DIR/$SESSION_NAME"
