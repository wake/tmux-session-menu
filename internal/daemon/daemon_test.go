package daemon

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/version"
)

// shortTempDir 建立短路徑暫存目錄（Unix socket 路徑限制 ~104 bytes）。
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tsm-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{DataDir: shortTempDir(t)}
}

// --- socketAlive ---

func TestSocketAlive_ActiveListener(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := dir + "/test.sock"

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	if !socketAlive(sockPath) {
		t.Error("expected socketAlive=true for active listener")
	}
}

func TestSocketAlive_StaleFile(t *testing.T) {
	dir := shortTempDir(t)
	sockPath := dir + "/test.sock"

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	lis.Close()

	if socketAlive(sockPath) {
		t.Error("expected socketAlive=false for stale socket file")
	}
}

func TestSocketAlive_NoFile(t *testing.T) {
	if socketAlive("/nonexistent/path/test.sock") {
		t.Error("expected socketAlive=false for nonexistent path")
	}
}

// --- Run: socket-based guard ---

func TestRun_RejectsWhenSocketActive(t *testing.T) {
	cfg := testConfig(t)
	sockPath := SocketPath(cfg)

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	d := NewDaemon(cfg)
	err = d.Run()
	if err == nil {
		t.Fatal("expected Run() to return error when socket is active")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' in error, got: %v", err)
	}
}

func TestRun_CleansStaleSocketAndStarts(t *testing.T) {
	cfg := testConfig(t)
	sockPath := SocketPath(cfg)

	// 建立殘留檔案（模擬 daemon 異常退出後留下的 socket 檔案）
	if err := os.WriteFile(sockPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}

	d := NewDaemon(cfg)
	go func() {
		for i := 0; i < 50; i++ {
			time.Sleep(50 * time.Millisecond)
			if socketAlive(sockPath) {
				d.Shutdown()
				return
			}
		}
		d.Shutdown()
	}()

	runErr := d.Run()
	if runErr != nil {
		t.Fatalf("expected Run() to succeed after cleaning stale socket, got: %v", runErr)
	}
}

// --- version file ---

func TestRun_WritesVersionFile(t *testing.T) {
	cfg := testConfig(t)
	sockPath := SocketPath(cfg)
	verPath := VersionPath(cfg)

	existedCh := make(chan bool, 1)
	d := NewDaemon(cfg)
	go func() {
		for i := 0; i < 50; i++ {
			time.Sleep(50 * time.Millisecond)
			if socketAlive(sockPath) {
				// 確認 daemon 運行中 version file 存在
				_, statErr := os.Stat(verPath)
				existedCh <- statErr == nil
				d.Shutdown()
				return
			}
		}
		existedCh <- false
		d.Shutdown()
	}()

	err := d.Run()
	require.NoError(t, err)

	assert.True(t, <-existedCh, "version file 應在 daemon 運行中存在")

	// Run 結束後 version file 應已被清理
	_, err = os.Stat(verPath)
	assert.True(t, os.IsNotExist(err), "version file 應在 daemon 結束後被清理")
}

func TestRun_VersionFileContent(t *testing.T) {
	cfg := testConfig(t)
	sockPath := SocketPath(cfg)
	verPath := VersionPath(cfg)

	d := NewDaemon(cfg)
	verCh := make(chan string, 1)
	go func() {
		for i := 0; i < 50; i++ {
			time.Sleep(50 * time.Millisecond)
			if socketAlive(sockPath) {
				// daemon 運行中讀取 version file
				data, err := os.ReadFile(verPath)
				if err == nil {
					verCh <- strings.TrimSpace(string(data))
				} else {
					verCh <- ""
				}
				d.Shutdown()
				return
			}
		}
		verCh <- ""
		d.Shutdown()
	}()

	err := d.Run()
	require.NoError(t, err)
	verContent := <-verCh
	assert.Equal(t, version.String(), verContent, "version file 應包含目前版本")
}

func TestStatus_IncludesVersion(t *testing.T) {
	cfg := testConfig(t)
	pidPath := PidPath(cfg)
	verPath := VersionPath(cfg)

	// 模擬 daemon 在執行中：寫入 pid 和 version 檔案
	require.NoError(t, os.MkdirAll(DataDir(cfg), 0o755))
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600))
	require.NoError(t, os.WriteFile(verPath, []byte("0.30.1"), 0o600))

	status, err := Status(cfg)
	require.NoError(t, err)
	assert.Contains(t, status, "0.30.1", "status 應包含版本資訊")
}

func TestStatus_NoVersionFile(t *testing.T) {
	cfg := testConfig(t)
	pidPath := PidPath(cfg)

	// 模擬舊版 daemon（無 version 檔案）
	require.NoError(t, os.MkdirAll(DataDir(cfg), 0o755))
	require.NoError(t, os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600))

	status, err := Status(cfg)
	require.NoError(t, err)
	// 應正常回傳，不因缺少 version file 而錯誤
	assert.Contains(t, status, fmt.Sprintf("pid=%d", os.Getpid()))
}
