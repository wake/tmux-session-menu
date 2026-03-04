package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// Daemon 是 tsm daemon 的主體。
type Daemon struct {
	cfg       config.Config
	server    *grpc.Server
	hub       *WatcherHub
	state     *StateManager
	store     *store.Store
	cancelRun context.CancelFunc
}

// NewDaemon 建立新的 Daemon。
func NewDaemon(cfg config.Config) *Daemon {
	return &Daemon{cfg: cfg}
}

// Run 啟動 daemon：建立 unix socket、啟動 gRPC server、開始狀態掃描。
// 會阻塞直到收到 SIGTERM/SIGINT。
func (d *Daemon) Run() error {
	dataDir := DataDir(d.cfg)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	sockPath := SocketPath(d.cfg)
	pidPath := PidPath(d.cfg)

	// 檢查是否已有 daemon 在執行
	if isRunning(pidPath) {
		return fmt.Errorf("daemon already running (pid file: %s)", pidPath)
	}

	// 清理舊 socket
	os.Remove(sockPath)

	// 寫入 PID 檔案
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	// 建立 unix socket listener
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	// 限制 socket 權限為僅限本人存取
	if err := os.Chmod(sockPath, 0o600); err != nil {
		lis.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	defer func() {
		lis.Close()
		os.Remove(sockPath)
	}()

	// 開啟 store
	dbPath := filepath.Join(dataDir, "state.db")
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	d.store = st
	defer st.Close()

	// 建立元件
	exec := tmux.NewRealExecutor()
	mgr := tmux.NewManager(exec)
	statusDir := filepath.Join(dataDir, "status")

	d.hub = NewWatcherHub()
	defer d.hub.Close()

	d.state = NewStateManager(mgr, st, d.cfg, statusDir, d.hub)
	svc := NewService(mgr, st, d.hub, d.state)

	d.server = grpc.NewServer()
	tsmv1.RegisterSessionManagerServer(d.server, svc)

	// 啟動狀態掃描迴圈
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRun = cancel
	go d.state.Run(ctx)

	// 監聽系統信號
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		d.Shutdown()
	}()

	fmt.Fprintf(os.Stderr, "tsm daemon started (pid=%d, socket=%s)\n", os.Getpid(), sockPath)
	if err := d.server.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// Shutdown 優雅停止 daemon。
func (d *Daemon) Shutdown() {
	if d.cancelRun != nil {
		d.cancelRun()
	}
	if d.server != nil {
		d.server.GracefulStop()
	}
}

// Start 以 re-exec 模式在背景啟動 daemon。
func Start(cfg config.Config) error {
	pidPath := PidPath(cfg)
	if isRunning(pidPath) {
		return fmt.Errorf("daemon already running (pid file: %s)", pidPath)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	cmd := exec.Command(exe, "daemon", "start", "--foreground")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon process: %w", err)
	}
	go cmd.Wait()

	// 輪詢等待 socket 就緒
	sockPath := SocketPath(cfg)
	ready := false
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if conn, err := net.Dial("unix", sockPath); err == nil {
			conn.Close()
			ready = true
			break
		}
	}
	if !ready {
		return fmt.Errorf("daemon did not become ready within 3s (socket: %s)", sockPath)
	}

	// 讀取 PID 檔案
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("daemon started but cannot read pid file: %w", err)
	}
	pid := strings.TrimSpace(string(data))

	fmt.Fprintf(os.Stderr, "tsm daemon started (pid=%s, socket=%s)\n", pid, sockPath)
	return nil
}

// Stop 停止由 PID 檔案指定的 daemon。
func Stop(cfg config.Config) error {
	pidPath := PidPath(cfg)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("read pid file: %w (daemon might not be running)", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("parse pid: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// 程序可能已終止，清理 PID 檔案
		os.Remove(pidPath)
		return fmt.Errorf("send SIGTERM to pid %d: %w", pid, err)
	}

	fmt.Fprintf(os.Stderr, "sent SIGTERM to daemon (pid=%d)\n", pid)
	return nil
}

// Status 回傳 daemon 的狀態資訊字串。
func Status(cfg config.Config) (string, error) {
	pidPath := PidPath(cfg)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return "", fmt.Errorf("daemon is not running (no pid file)")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return "", fmt.Errorf("invalid pid file")
	}

	if !processExists(pid) {
		os.Remove(pidPath)
		return "", fmt.Errorf("daemon is not running (stale pid file cleaned)")
	}

	sockPath := SocketPath(cfg)
	return fmt.Sprintf("daemon running (pid=%d, socket=%s)", pid, sockPath), nil
}

// isRunning 檢查 PID 檔案中的程序是否仍在執行。
func isRunning(pidPath string) bool {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return processExists(pid)
}

// processExists 檢查指定 PID 的程序是否存在。
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// 用 signal 0 偵測程序是否存在
	return proc.Signal(syscall.Signal(0)) == nil
}
