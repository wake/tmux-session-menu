package selfinstall

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/version"
)

// saveAndRestore 儲存並在測試結束後還原 package-level 函式。
func saveAndRestore(t *testing.T) {
	origLookPath := lookPathFn
	origRunVersion := runVersionFn
	origGoPathBin := goPathBinFn
	t.Cleanup(func() {
		lookPathFn = origLookPath
		runVersionFn = origRunVersion
		goPathBinFn = origGoPathBin
	})
}

func TestDetectNotFound(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "", errors.New("not found")
	}

	r := Detect()
	assert.Equal(t, StatusNotFound, r.Status)
	assert.Empty(t, r.Path)
}

func TestDetectConflict_ExecError(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "", errors.New("exec error")
	}

	r := Detect()
	assert.Equal(t, StatusConflict, r.Status)
	assert.Equal(t, "/usr/bin/tsm", r.Path)
}

func TestDetectConflict_WrongPrefix(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "something else 1.0.0", nil
	}

	r := Detect()
	assert.Equal(t, StatusConflict, r.Status)
	assert.Equal(t, "/usr/bin/tsm", r.Path)
}

func TestDetectOutdated(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/local/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "tsm old-version", nil
	}

	r := Detect()
	assert.Equal(t, StatusOutdated, r.Status)
	assert.Equal(t, "/usr/local/bin/tsm", r.Path)
	assert.Equal(t, "old-version", r.Version)
}

func TestDetectSameVersion(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/local/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "tsm " + version.String(), nil
	}

	r := Detect()
	assert.Equal(t, StatusSameVersion, r.Status)
	assert.Equal(t, "/usr/local/bin/tsm", r.Path)
	assert.Equal(t, version.String(), r.Version)
}

func TestBuildComponentNotFound(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "", errors.New("not found")
	}
	goPathBinFn = func() string { return "/mock/go/bin" }

	comp := BuildComponent()
	assert.Contains(t, comp.Label, "安裝 tsm 到")
	assert.Contains(t, comp.Label, "/mock/go/bin/tsm")
	assert.True(t, comp.Checked)
	assert.False(t, comp.Disabled)
}

func TestBuildComponentConflict(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "other-tool 2.0", nil
	}

	comp := BuildComponent()
	assert.Contains(t, comp.Label, "衝突")
	assert.False(t, comp.Checked)
	assert.True(t, comp.Disabled)
	assert.Contains(t, comp.Note, "/usr/bin/tsm")
}

func TestBuildComponentOutdated(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/local/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "tsm old-ver", nil
	}

	comp := BuildComponent()
	assert.Contains(t, comp.Label, "更新 tsm 到")
	assert.True(t, comp.Checked)
	assert.False(t, comp.Disabled)
	assert.Contains(t, comp.Note, "old-ver")
}

func TestBuildComponentSameVersion(t *testing.T) {
	saveAndRestore(t)
	lookPathFn = func(string) (string, error) {
		return "/usr/local/bin/tsm", nil
	}
	runVersionFn = func(string) (string, error) {
		return "tsm " + version.String(), nil
	}

	comp := BuildComponent()
	assert.Contains(t, comp.Label, "覆蓋安裝 tsm 到")
	assert.False(t, comp.Checked)
	assert.False(t, comp.Disabled)
	assert.Contains(t, comp.Note, version.String())
}

func TestInstallCopiesFile(t *testing.T) {
	// 建立模擬來源檔
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "tsm-src")
	content := []byte("#!/bin/sh\necho mock binary")
	require.NoError(t, os.WriteFile(srcFile, content, 0o755))

	// 目標目錄
	targetDir := t.TempDir()

	// 暫時替換 os.Executable 的結果（透過直接呼叫 Install 前先複製）
	// 由於 Install 使用 os.Executable，這裡直接測試 copyFile
	target := filepath.Join(targetDir, "tsm-copy")
	err := copyFile(srcFile, target)
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// 驗證執行權限
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0o111, "應有執行權限")
}
