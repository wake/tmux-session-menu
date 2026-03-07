package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreDetect 儲存並在測試結束後還原 package-level 函式。
func saveAndRestoreDetect(t *testing.T) {
	origSettings := settingsPathFn
	origScript := scriptPathFn
	t.Cleanup(func() {
		settingsPathFn = origSettings
		scriptPathFn = origScript
	})
}

// ─── Detect ──────────────────────────────────────────────────

func TestDetect_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{}`), 0644)

	mgr := NewManager(p, "/usr/local/bin/tsm-hook.sh")
	r := mgr.Detect()

	assert.Equal(t, HooksNotInstalled, r.Status)
	assert.Empty(t, r.InstalledEvents)
	assert.Len(t, r.MissingEvents, len(tsmEvents))
}

func TestDetect_Installed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{}`), 0644)

	mgr := NewManager(p, "/usr/local/bin/tsm-hook.sh")

	// 安裝所有 hooks
	_, err := mgr.Install(false)
	require.NoError(t, err)

	r := mgr.Detect()
	assert.Equal(t, HooksInstalled, r.Status)
	assert.Len(t, r.InstalledEvents, len(tsmEvents))
	assert.Empty(t, r.MissingEvents)
}

func TestDetect_Partial(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte(`{}`), 0644)

	mgr := NewManager(p, "/usr/local/bin/tsm-hook.sh")

	// 安裝所有 hooks
	_, err := mgr.Install(false)
	require.NoError(t, err)

	// 手動移除一個 event 的 hook
	data, _ := os.ReadFile(p)
	var settings map[string]interface{}
	json.Unmarshal(data, &settings)
	hooksMap := settings["hooks"].(map[string]interface{})
	delete(hooksMap, "Stop")
	newData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(p, newData, 0644)

	r := mgr.Detect()
	assert.Equal(t, HooksPartial, r.Status)
	assert.Len(t, r.InstalledEvents, len(tsmEvents)-1)
	assert.Len(t, r.MissingEvents, 1)
	assert.Contains(t, r.MissingEvents, "Stop")
}

func TestDetect_NoSettings(t *testing.T) {
	mgr := NewManager("/nonexistent/path/settings.json", "/usr/local/bin/tsm-hook.sh")

	r := mgr.Detect()
	assert.Equal(t, HooksNoSettings, r.Status)
}

// ─── BuildComponent ─────────────────────────────────────────

func TestBuildComponent_NotInstalled(t *testing.T) {
	saveAndRestoreDetect(t)

	dir := t.TempDir()
	sp := filepath.Join(dir, "settings.json")
	os.WriteFile(sp, []byte(`{}`), 0644)

	settingsPathFn = func() string { return sp }
	scriptPathFn = func() string { return filepath.Join(dir, "tsm-hook.sh") }

	comp := BuildComponent()
	assert.True(t, comp.Checked)
	assert.False(t, comp.Disabled)
	assert.Equal(t, "Claude Code hooks", comp.Label)
	assert.Contains(t, comp.Note, "將寫入")
	assert.Contains(t, comp.Note, "settings.json")
}

func TestBuildComponent_NoSettings_HasNote(t *testing.T) {
	saveAndRestoreDetect(t)

	dir := t.TempDir()
	settingsPathFn = func() string { return filepath.Join(dir, "nonexistent", "settings.json") }
	scriptPathFn = func() string { return filepath.Join(dir, "tsm-hook.sh") }

	comp := BuildComponent()
	assert.Contains(t, comp.Note, "將寫入")
}

func TestBuildComponent_Installed(t *testing.T) {
	saveAndRestoreDetect(t)

	dir := t.TempDir()
	sp := filepath.Join(dir, "settings.json")
	os.WriteFile(sp, []byte(`{}`), 0644)

	scriptPath := filepath.Join(dir, "tsm-hook.sh")
	settingsPathFn = func() string { return sp }
	scriptPathFn = func() string { return scriptPath }

	// 先安裝
	mgr := NewManager(sp, scriptPath)
	_, err := mgr.Install(false)
	require.NoError(t, err)

	comp := BuildComponent()
	assert.False(t, comp.Checked)
	assert.False(t, comp.Disabled)
	assert.Contains(t, comp.Note, "已安裝")
	assert.Contains(t, comp.Note, "6/6")
}

func TestBuildComponent_Partial(t *testing.T) {
	saveAndRestoreDetect(t)

	dir := t.TempDir()
	sp := filepath.Join(dir, "settings.json")
	os.WriteFile(sp, []byte(`{}`), 0644)

	scriptPath := filepath.Join(dir, "tsm-hook.sh")
	settingsPathFn = func() string { return sp }
	scriptPathFn = func() string { return scriptPath }

	// 安裝後移除一個 event
	mgr := NewManager(sp, scriptPath)
	_, err := mgr.Install(false)
	require.NoError(t, err)

	data, _ := os.ReadFile(sp)
	var settings map[string]interface{}
	json.Unmarshal(data, &settings)
	hooksMap := settings["hooks"].(map[string]interface{})
	delete(hooksMap, "Stop")
	newData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(sp, newData, 0644)

	comp := BuildComponent()
	assert.True(t, comp.Checked)
	assert.False(t, comp.Disabled)
	assert.Contains(t, comp.Note, "部分安裝")
	assert.Contains(t, comp.Note, "5/6")
}

func TestBuildComponent_NoSettings(t *testing.T) {
	saveAndRestoreDetect(t)

	dir := t.TempDir()
	settingsPathFn = func() string { return filepath.Join(dir, "nonexistent", "settings.json") }
	scriptPathFn = func() string { return filepath.Join(dir, "tsm-hook.sh") }

	comp := BuildComponent()
	// HooksNoSettings → Checked: true (treat as not installed)
	assert.True(t, comp.Checked)
	assert.False(t, comp.Disabled)
}

func TestBuildComponent_EmptyHome(t *testing.T) {
	saveAndRestoreDetect(t)

	settingsPathFn = func() string { return "" }
	scriptPathFn = func() string { return "" }

	comp := BuildComponent()
	assert.False(t, comp.Checked)
	assert.True(t, comp.Disabled)
	assert.Contains(t, comp.Note, "home")
}
