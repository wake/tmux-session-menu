#!/usr/bin/env bash
# tsm-hook_test.sh — Smoke tests for tsm-hook.sh
#
# Runs without tmux by injecting a mock `tmux` command into PATH.
# Each test verifies the output JSON written by the hook script.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
HOOK_SCRIPT="$REPO_ROOT/internal/hooks/tsm-hook.sh"

PASS_COUNT=0
FAIL_COUNT=0

# --- Helpers ------------------------------------------------------------------

setup_env() {
  ORIG_PATH="$PATH"
  # Create a fresh temp dir for each test
  TEST_TMP="$(mktemp -d)"
  export TSM_STATUS_DIR="$TEST_TMP/status"

  # Create a mock tmux that returns a known session name
  MOCK_BIN="$TEST_TMP/bin"
  mkdir -p "$MOCK_BIN"
  cat > "$MOCK_BIN/tmux" <<'MOCK'
#!/usr/bin/env bash
echo "test-session"
MOCK
  chmod +x "$MOCK_BIN/tmux"

  # Put mock tmux first in PATH and set TMUX to simulate being inside tmux
  export PATH="$MOCK_BIN:$PATH"
  export TMUX="/tmp/tmux-fake/default,12345,0"
}

teardown_env() {
  rm -rf "$TEST_TMP"
  export PATH="$ORIG_PATH"
}

assert_status() {
  local test_name="$1"
  local expected_status="$2"
  local expected_event="$3"
  local expected_ai_type="$4"
  local file="$TSM_STATUS_DIR/test-session"

  if [[ ! -f "$file" ]]; then
    echo "FAIL: $test_name — status file not found at $file"
    FAIL_COUNT=$((FAIL_COUNT + 1))
    return
  fi

  local result
  result="$(python3 - "$file" "$expected_status" "$expected_event" "$expected_ai_type" <<'PYEOF'
import json, sys

filepath, exp_status, exp_event, exp_ai_type = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
with open(filepath) as f:
    d = json.load(f)

errors = []
if d.get("status") != exp_status:
    errors.append("status: expected %r, got %r" % (exp_status, d.get("status")))
if d.get("event") != exp_event:
    errors.append("event: expected %r, got %r" % (exp_event, d.get("event")))
if not isinstance(d.get("timestamp"), int) or d["timestamp"] < 1000000000:
    errors.append("timestamp: invalid value %r" % (d.get("timestamp"),))
if d.get("ai_type", "") != exp_ai_type:
    errors.append("ai_type: expected %r, got %r" % (exp_ai_type, d.get("ai_type", "")))
if errors:
    print("ERRORS: " + "; ".join(errors))
else:
    print("OK")
PYEOF
  )"

  if [[ "$result" == "OK" ]]; then
    echo "PASS: $test_name"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "FAIL: $test_name — $result"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

assert_no_file() {
  local test_name="$1"
  local file="$TSM_STATUS_DIR/test-session"

  if [[ ! -f "$file" ]]; then
    echo "PASS: $test_name"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    echo "FAIL: $test_name — file should not exist but found at $file"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

# --- Test Cases ---------------------------------------------------------------

echo "=== tsm-hook.sh smoke tests ==="
echo ""

# Test 1: UserPromptSubmit -> running
setup_env
echo '{"hook_event_name":"UserPromptSubmit"}' | bash "$HOOK_SCRIPT"
assert_status "UserPromptSubmit -> running" "running" "UserPromptSubmit" "claude"
teardown_env

# Test 2: Stop -> idle
setup_env
echo '{"hook_event_name":"Stop"}' | bash "$HOOK_SCRIPT"
assert_status "Stop -> idle" "idle" "Stop" "claude"
teardown_env

# Test 3: No TMUX env -> no file written
setup_env
unset TMUX
echo '{"hook_event_name":"UserPromptSubmit"}' | bash "$HOOK_SCRIPT"
assert_no_file "No TMUX env -> no file written"
teardown_env

# Test 4: PermissionRequest -> waiting
setup_env
echo '{"hook_event_name":"PermissionRequest"}' | bash "$HOOK_SCRIPT"
assert_status "PermissionRequest -> waiting" "waiting" "PermissionRequest" "claude"
teardown_env

# Test 5: SessionStart -> running
setup_env
echo '{"hook_event_name":"SessionStart"}' | bash "$HOOK_SCRIPT"
assert_status "SessionStart -> running" "running" "SessionStart" "claude"
teardown_env

# Test 6: SessionEnd -> idle
setup_env
echo '{"hook_event_name":"SessionEnd"}' | bash "$HOOK_SCRIPT"
assert_status "SessionEnd -> idle" "idle" "SessionEnd" ""
teardown_env

# Test 7: Notification -> waiting
setup_env
echo '{"hook_event_name":"Notification"}' | bash "$HOOK_SCRIPT"
assert_status "Notification -> waiting" "waiting" "Notification" "claude"
teardown_env

# Test 8: Unknown event -> running (default)
setup_env
echo '{"hook_event_name":"SomeUnknownEvent"}' | bash "$HOOK_SCRIPT"
assert_status "Unknown event -> running (default)" "running" "SomeUnknownEvent" "claude"
teardown_env

# Test 9: Empty/invalid JSON -> no file written (graceful handling)
setup_env
echo '' | bash "$HOOK_SCRIPT"
assert_no_file "Empty input -> no file written"
teardown_env

# --- Summary ------------------------------------------------------------------

echo ""
echo "=== Results: $PASS_COUNT passed, $FAIL_COUNT failed ==="

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  exit 1
fi

exit 0
