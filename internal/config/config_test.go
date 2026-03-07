package config_test

import (
	"os"
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
