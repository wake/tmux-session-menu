package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.Default()

	assert.Equal(t, "~/.config/tsm", cfg.DataDir)
	assert.Equal(t, 150, cfg.PreviewLines)
	assert.Equal(t, 2, cfg.PollIntervalSec)
}

func TestLoadFromTOML(t *testing.T) {
	tomlData := `
data_dir = "/tmp/tsm-test"
preview_lines = 50
poll_interval_sec = 5
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/tsm-test", cfg.DataDir)
	assert.Equal(t, 50, cfg.PreviewLines)
	assert.Equal(t, 5, cfg.PollIntervalSec)
}

func TestLoadFromTOML_PartialOverride(t *testing.T) {
	tomlData := `preview_lines = 80`

	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "~/.config/tsm", cfg.DataDir)
	assert.Equal(t, 80, cfg.PreviewLines)
	assert.Equal(t, 2, cfg.PollIntervalSec)
}

func TestLoadFromTOML_InvalidInput(t *testing.T) {
	_, err := config.LoadFromString("invalid {{{")
	assert.Error(t, err)
}

// --- InstallMode 測試 ---

func TestInstallMode_Constants(t *testing.T) {
	assert.Equal(t, config.InstallMode("full"), config.ModeFull)
	assert.Equal(t, config.InstallMode("client"), config.ModeClient)
}

func TestSaveAndLoadInstallMode(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveInstallMode(dir, config.ModeFull)
	require.NoError(t, err)

	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeFull, mode)
}

func TestSaveAndLoadInstallMode_Client(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveInstallMode(dir, config.ModeClient)
	require.NoError(t, err)

	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeClient, mode)
}

func TestLoadInstallMode_NoFile_DefaultsFull(t *testing.T) {
	dir := t.TempDir()
	mode := config.LoadInstallMode(dir)
	assert.Equal(t, config.ModeFull, mode)
}

// --- LastRemoteHost 測試 ---

func TestSaveAndLoadLastRemoteHost(t *testing.T) {
	dir := t.TempDir()

	err := config.SaveLastRemoteHost(dir, "myserver.local")
	require.NoError(t, err)

	host := config.LoadLastRemoteHost(dir)
	assert.Equal(t, "myserver.local", host)
}

func TestLoadLastRemoteHost_NoFile_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	host := config.LoadLastRemoteHost(dir)
	assert.Equal(t, "", host)
}

// --- ColorConfig 測試 ---

func TestDefaultConfig_HasColorSections(t *testing.T) {
	cfg := config.Default()

	// Local 預設色值
	assert.Equal(t, "", cfg.Local.BarBG, "local bar_bg 預設為空（保持 tmux 預設）")
	assert.Equal(t, "#5f8787", cfg.Local.BadgeBG, "local badge_bg 預設值")
	assert.Equal(t, "#c0caf5", cfg.Local.BadgeFG, "local badge_fg 預設值")

	// Remote 預設色值
	assert.Equal(t, "#1a2b2b", cfg.Remote.BarBG, "remote bar_bg 預設值")
	assert.Equal(t, "#73daca", cfg.Remote.BadgeBG, "remote badge_bg 預設值")
	assert.Equal(t, "#1a1b26", cfg.Remote.BadgeFG, "remote badge_fg 預設值")
}

func TestLoadFromTOML_WithColorSections(t *testing.T) {
	tomlData := `
data_dir = "/tmp/tsm-test"

[local]
bar_bg = "#111111"
badge_bg = "#222222"
badge_fg = "#333333"

[remote]
bar_bg = "#444444"
badge_bg = "#555555"
badge_fg = "#666666"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/tsm-test", cfg.DataDir)

	assert.Equal(t, "#111111", cfg.Local.BarBG)
	assert.Equal(t, "#222222", cfg.Local.BadgeBG)
	assert.Equal(t, "#333333", cfg.Local.BadgeFG)

	assert.Equal(t, "#444444", cfg.Remote.BarBG)
	assert.Equal(t, "#555555", cfg.Remote.BadgeBG)
	assert.Equal(t, "#666666", cfg.Remote.BadgeFG)
}

func TestLoadFromTOML_PartialColors_KeepsDefaults(t *testing.T) {
	tomlData := `
[local]
badge_bg = "#aabbcc"

[remote]
bar_bg = "#ddeeff"
`
	cfg, err := config.LoadFromString(tomlData)
	require.NoError(t, err)

	// Local: 只覆蓋 badge_bg，其餘保持預設
	assert.Equal(t, "", cfg.Local.BarBG, "未設定的 local bar_bg 保持預設空值")
	assert.Equal(t, "#aabbcc", cfg.Local.BadgeBG, "已設定的 local badge_bg 為覆蓋值")
	assert.Equal(t, "#c0caf5", cfg.Local.BadgeFG, "未設定的 local badge_fg 保持預設")

	// Remote: 只覆蓋 bar_bg，其餘保持預設
	assert.Equal(t, "#ddeeff", cfg.Remote.BarBG, "已設定的 remote bar_bg 為覆蓋值")
	assert.Equal(t, "#73daca", cfg.Remote.BadgeBG, "未設定的 remote badge_bg 保持預設")
	assert.Equal(t, "#1a1b26", cfg.Remote.BadgeFG, "未設定的 remote badge_fg 保持預設")
}

// --- SaveConfig 測試 ---

func TestSaveConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.PreviewLines = 200
	cfg.Local.BarBG = "#aabbcc"

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	// 重新讀取驗證
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	loaded, err := config.LoadFromString(string(data))
	require.NoError(t, err)

	assert.Equal(t, 200, loaded.PreviewLines)
	assert.Equal(t, "#aabbcc", loaded.Local.BarBG)
	assert.Equal(t, "#5f8787", loaded.Local.BadgeBG)
}

func TestSaveConfig_RuntimeFieldsNotSaved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := config.Default()
	cfg.InTmux = true
	cfg.InPopup = true

	err := config.SaveConfig(path, cfg)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.NotContains(t, content, "in_tmux")
	assert.NotContains(t, content, "in_popup")
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.config/tsm", home + "/.config/tsm"},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := config.ExpandPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
