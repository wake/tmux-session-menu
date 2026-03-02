package hooks_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/hooks"
)

var allEvents = []string{
	"UserPromptSubmit",
	"Stop",
	"SessionStart",
	"SessionEnd",
	"PermissionRequest",
	"Notification",
}

// helper: 建立暫存 settings.json 並回傳路徑
func setupSettingsFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	err := os.WriteFile(p, []byte(content), 0644)
	require.NoError(t, err)
	return p
}

// helper: 讀取 settings.json 並解析為 map
func readSettings(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var m map[string]interface{}
	err = json.Unmarshal(data, &m)
	require.NoError(t, err)
	return m
}

// ─── Task 3: Install ────────────────────────────────────────

func TestInstall_EmptySettings(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	result, err := m.Install(true) // dry-run
	require.NoError(t, err)

	assert.True(t, result.Changed)
	assert.ElementsMatch(t, allEvents, result.EventsAdded)
	assert.Empty(t, result.EventsRemoved)

	// dry-run 不應該寫入檔案
	raw, _ := os.ReadFile(p)
	assert.Equal(t, `{}`, string(raw))

	// NewSettings 應包含有效 JSON
	assert.NotEmpty(t, result.NewSettings)
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(result.NewSettings), &parsed)
	require.NoError(t, err)
}

func TestInstall_Apply(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	result, err := m.Install(false) // 實際寫入
	require.NoError(t, err)
	assert.True(t, result.Changed)

	settings := readSettings(t, p)
	hooksMap, ok := settings["hooks"].(map[string]interface{})
	require.True(t, ok, "hooks should be a map")

	for _, event := range allEvents {
		entries, ok := hooksMap[event].([]interface{})
		require.True(t, ok, "event %s should be an array", event)
		assert.Len(t, entries, 1, "event %s should have exactly 1 entry", event)

		// 驗證 matcher group 結構
		entry := entries[0].(map[string]interface{})
		assert.Equal(t, "", entry["matcher"])

		innerHooks := entry["hooks"].([]interface{})
		assert.Len(t, innerHooks, 1)
		hook := innerHooks[0].(map[string]interface{})
		assert.Equal(t, "command", hook["type"])
		assert.Equal(t, "/usr/local/bin/tsm-hook.sh", hook["command"])
	}
}

func TestInstall_PreservesExistingHooks(t *testing.T) {
	// Stop 已經有一個其他 hook
	initial := `{
  "hooks": {
    "Stop": [
      {
        "matcher": "*.py",
        "hooks": [
          {"type": "command", "command": "/usr/bin/other-hook.sh"}
        ]
      }
    ]
  }
}`
	p := setupSettingsFile(t, initial)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	result, err := m.Install(false)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	settings := readSettings(t, p)
	hooksMap := settings["hooks"].(map[string]interface{})
	stopEntries := hooksMap["Stop"].([]interface{})

	// Stop 應該有 2 個 matcher group：原有的 + tsm 新增的
	assert.Len(t, stopEntries, 2)

	// 第一個應該是原有的
	first := stopEntries[0].(map[string]interface{})
	firstHooks := first["hooks"].([]interface{})
	firstCmd := firstHooks[0].(map[string]interface{})["command"]
	assert.Equal(t, "/usr/bin/other-hook.sh", firstCmd)

	// 第二個應該是 tsm 的
	second := stopEntries[1].(map[string]interface{})
	secondHooks := second["hooks"].([]interface{})
	secondCmd := secondHooks[0].(map[string]interface{})["command"]
	assert.Equal(t, "/usr/local/bin/tsm-hook.sh", secondCmd)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	// 第一次安裝
	_, err := m.Install(false)
	require.NoError(t, err)

	// 第二次安裝 — 不應有變更
	result, err := m.Install(false)
	require.NoError(t, err)
	assert.False(t, result.Changed)
	assert.Empty(t, result.EventsAdded)
}

// ─── Task 4: Uninstall ──────────────────────────────────────

func TestUninstall_RemovesTSMHooks(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	// 先安裝
	_, err := m.Install(false)
	require.NoError(t, err)

	// 再移除
	result, err := m.Uninstall(false)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.ElementsMatch(t, allEvents, result.EventsRemoved)

	// hooks map 下不應有任何 event key（因為都清空了）
	settings := readSettings(t, p)
	hooksMap, ok := settings["hooks"].(map[string]interface{})
	if ok {
		assert.Empty(t, hooksMap, "all event keys should be removed when empty")
	}
}

func TestUninstall_PreservesOtherHooks(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	// 安裝 tsm hooks
	_, err := m.Install(false)
	require.NoError(t, err)

	// 手動加一個其他 hook 到 Stop
	settings := readSettings(t, p)
	hooksMap := settings["hooks"].(map[string]interface{})
	stopEntries := hooksMap["Stop"].([]interface{})
	otherEntry := map[string]interface{}{
		"matcher": "*.py",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": "/usr/bin/other-hook.sh",
			},
		},
	}
	stopEntries = append(stopEntries, otherEntry)
	hooksMap["Stop"] = stopEntries
	data, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(p, data, 0644)

	// 移除 tsm hooks
	result, err := m.Uninstall(false)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// Stop 應該還保留其他 hook
	settings = readSettings(t, p)
	hooksMap = settings["hooks"].(map[string]interface{})
	stopEntries2 := hooksMap["Stop"].([]interface{})
	assert.Len(t, stopEntries2, 1)
	entry := stopEntries2[0].(map[string]interface{})
	entryHooks := entry["hooks"].([]interface{})
	cmd := entryHooks[0].(map[string]interface{})["command"]
	assert.Equal(t, "/usr/bin/other-hook.sh", cmd)

	// 其他 event key 因為全空了，不應存在
	for _, event := range allEvents {
		if event == "Stop" {
			continue
		}
		_, exists := hooksMap[event]
		assert.False(t, exists, "event %s should be removed (empty)", event)
	}
}

func TestUninstall_NothingToRemove(t *testing.T) {
	p := setupSettingsFile(t, `{}`)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	result, err := m.Uninstall(false)
	require.NoError(t, err)
	assert.False(t, result.Changed)
	assert.Empty(t, result.EventsRemoved)
}

// ─── Task 5: Field Preservation ─────────────────────────────

func TestInstall_PreservesNonHookFields(t *testing.T) {
	initial := `{
  "includeCoAuthoredBy": true,
  "statusLine": "compact",
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "",
        "hooks": [
          {"type": "command", "command": "/usr/bin/pre-tool.sh"}
        ]
      }
    ]
  }
}`
	p := setupSettingsFile(t, initial)
	m := hooks.NewManager(p, "/usr/local/bin/tsm-hook.sh")

	_, err := m.Install(false)
	require.NoError(t, err)

	settings := readSettings(t, p)

	// 非 hooks 欄位應完整保留
	assert.Equal(t, true, settings["includeCoAuthoredBy"])
	assert.Equal(t, "compact", settings["statusLine"])

	// PreToolUse hook 應完整保留（tsm 不管理此 event）
	hooksMap := settings["hooks"].(map[string]interface{})
	preToolUse := hooksMap["PreToolUse"].([]interface{})
	assert.Len(t, preToolUse, 1)
	entry := preToolUse[0].(map[string]interface{})
	entryHooks := entry["hooks"].([]interface{})
	cmd := entryHooks[0].(map[string]interface{})["command"]
	assert.Equal(t, "/usr/bin/pre-tool.sh", cmd)

	// tsm 管理的 6 個 event 也要在
	for _, event := range allEvents {
		_, ok := hooksMap[event]
		assert.True(t, ok, "event %s should be present", event)
	}
}
