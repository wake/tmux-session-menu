package tmux_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

func TestReadHookStatus(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "my-session")
	content := fmt.Sprintf(`{"status":"running","timestamp":%d,"event":"UserPromptSubmit"}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hs, err := tmux.ReadHookStatus(dir, "my-session")
	require.NoError(t, err)
	assert.Equal(t, tmux.StatusRunning, hs.Status)
	assert.Equal(t, "UserPromptSubmit", hs.Event)
	assert.True(t, hs.IsValid())
}

func TestReadHookStatus_Expired(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "old-session")
	content := fmt.Sprintf(`{"status":"running","timestamp":%d,"event":"UserPromptSubmit"}`, time.Now().Add(-3*time.Minute).Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hs, err := tmux.ReadHookStatus(dir, "old-session")
	require.NoError(t, err)
	assert.False(t, hs.IsValid())
}

func TestReadHookStatus_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "bad-session")
	require.NoError(t, os.WriteFile(statusFile, []byte("{bad json"), 0644))

	_, err := tmux.ReadHookStatus(dir, "bad-session")
	assert.Error(t, err)
}

func TestReadHookStatus_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := tmux.ReadHookStatus(dir, "nonexistent")
	assert.Error(t, err)
}

func TestReadHookStatus_AiType(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "cc-session")
	content := fmt.Sprintf(`{"status":"running","timestamp":%d,"event":"UserPromptSubmit","ai_type":"claude"}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hs, err := tmux.ReadHookStatus(dir, "cc-session")
	require.NoError(t, err)
	assert.Equal(t, "claude", hs.AiType)
	assert.True(t, hs.IsAiPresent())
}

func TestReadHookStatus_AiType_Empty(t *testing.T) {
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "shell-session")
	content := fmt.Sprintf(`{"status":"idle","timestamp":%d,"event":"SessionEnd","ai_type":""}`, time.Now().Unix())
	require.NoError(t, os.WriteFile(statusFile, []byte(content), 0644))

	hs, err := tmux.ReadHookStatus(dir, "shell-session")
	require.NoError(t, err)
	assert.Equal(t, "", hs.AiType)
	assert.False(t, hs.IsAiPresent())
}
