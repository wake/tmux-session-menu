package upgrade

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSilent_AlreadyLatest(t *testing.T) {
	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			return Release{Version: "1.0.0"}, nil
		},
		// 其餘不應被呼叫
		Download:      func(url string) (string, error) { t.Fatal("unexpected Download"); return "", nil },
		InstallBinary: func(tmp string) (string, error) { t.Fatal("unexpected InstallBinary"); return "", nil },
		RemoveTmp:     func(path string) { t.Fatal("unexpected RemoveTmp") },
		StopDaemon:    func() error { t.Fatal("unexpected StopDaemon"); return nil },
		StartDaemon:   func() error { t.Fatal("unexpected StartDaemon"); return nil },
	}

	result, err := RunSilent(ops)
	require.NoError(t, err)
	assert.False(t, result.Upgraded)
	assert.Equal(t, "1.0.0", result.Version)
}

func TestRunSilent_UpgradeSuccess(t *testing.T) {
	var calls []string

	asset := fmt.Sprintf("tsm-%s-%s", runtime.GOOS, runtime.GOARCH)
	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			calls = append(calls, "check")
			return Release{
				Version: "1.1.0",
				Assets:  map[string]string{asset: "https://example.com/dl"},
			}, nil
		},
		Download: func(url string) (string, error) {
			calls = append(calls, "download")
			assert.Equal(t, "https://example.com/dl", url)
			return "/tmp/tsm-new", nil
		},
		InstallBinary: func(tmp string) (string, error) {
			calls = append(calls, "install:"+tmp)
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			calls = append(calls, "remove:"+path)
		},
		StopDaemon: func() error {
			calls = append(calls, "stop")
			return nil
		},
		StartDaemon: func() error {
			calls = append(calls, "start")
			return nil
		},
	}

	result, err := RunSilent(ops)
	require.NoError(t, err)
	assert.True(t, result.Upgraded)
	assert.Equal(t, "1.1.0", result.Version)
	assert.Equal(t, []string{
		"check",
		"download",
		"install:/tmp/tsm-new",
		"remove:/tmp/tsm-new",
		"stop",
		"start",
	}, calls)
}

func TestRunSilent_CheckLatestError(t *testing.T) {
	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			return Release{}, fmt.Errorf("network timeout")
		},
	}

	_, err := RunSilent(ops)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "檢查最新版本失敗")
	assert.Contains(t, err.Error(), "network timeout")
}

func TestRunSilent_NoAssetForPlatform(t *testing.T) {
	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			return Release{
				Version: "1.1.0",
				Assets:  map[string]string{"tsm-plan9-mips": "https://example.com/dl"},
			}, nil
		},
	}

	_, err := RunSilent(ops)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "找不到適用於此平台的檔案")
}

func TestRunSilent_DownloadError(t *testing.T) {
	asset := fmt.Sprintf("tsm-%s-%s", runtime.GOOS, runtime.GOARCH)
	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			return Release{
				Version: "1.1.0",
				Assets:  map[string]string{asset: "https://example.com/dl"},
			}, nil
		},
		Download: func(url string) (string, error) {
			return "", fmt.Errorf("connection refused")
		},
	}

	_, err := RunSilent(ops)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "下載失敗")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRunSilent_StartDaemonError(t *testing.T) {
	asset := fmt.Sprintf("tsm-%s-%s", runtime.GOOS, runtime.GOARCH)
	var removeCalled bool

	ops := SilentOps{
		CurrentVersion: "1.0.0",
		CheckLatest: func() (Release, error) {
			return Release{
				Version: "1.1.0",
				Assets:  map[string]string{asset: "https://example.com/dl"},
			}, nil
		},
		Download: func(url string) (string, error) {
			return "/tmp/tsm-new", nil
		},
		InstallBinary: func(tmp string) (string, error) {
			return "/usr/local/bin/tsm", nil
		},
		RemoveTmp: func(path string) {
			removeCalled = true
		},
		StopDaemon: func() error {
			return nil
		},
		StartDaemon: func() error {
			return fmt.Errorf("port in use")
		},
	}

	_, err := RunSilent(ops)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "啟動 daemon 失敗")
	assert.Contains(t, err.Error(), "port in use")
	assert.True(t, removeCalled, "RemoveTmp should be called even when StartDaemon fails")
}
