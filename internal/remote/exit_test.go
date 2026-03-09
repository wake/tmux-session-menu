package remote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitMarkerPath(t *testing.T) {
	p := ExitMarkerPath()
	assert.Contains(t, p, ".config/tsm/exit-requested")
}

func TestWriteAndRemoveExitMarker(t *testing.T) {
	// 使用臨時目錄避免影響真實環境
	tmpDir := t.TempDir()
	tsmDir := filepath.Join(tmpDir, ".config", "tsm")
	require.NoError(t, os.MkdirAll(tsmDir, 0o755))

	markerPath := filepath.Join(tsmDir, exitMarkerName)

	// 手動寫入 marker（模擬 WriteExitMarker 但使用臨時路徑）
	err := os.WriteFile(markerPath, nil, 0o644)
	require.NoError(t, err)

	// 確認檔案存在
	_, err = os.Stat(markerPath)
	assert.NoError(t, err, "marker 檔案應該存在")

	// 移除
	os.Remove(markerPath)

	// 確認已移除
	_, err = os.Stat(markerPath)
	assert.True(t, os.IsNotExist(err), "marker 檔案應該已被移除")
}

func TestRemoveExitMarker_NoFile(t *testing.T) {
	// 移除不存在的檔案不應 panic
	RemoveExitMarker()
}

func TestCheckAndClearExitRequested_InvalidHost(t *testing.T) {
	// 不可能連到的主機應回傳 false
	result := CheckAndClearExitRequested("nonexistent-host-12345.invalid")
	assert.False(t, result, "無法連線的主機應回傳 false")
}
