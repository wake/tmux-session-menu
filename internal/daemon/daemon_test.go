package daemon

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wake/tmux-session-menu/internal/config"
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
