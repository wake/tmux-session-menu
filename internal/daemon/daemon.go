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
	"github.com/wake/tmux-session-menu/internal/version"
)

// Daemon 是 tsm daemon 的主體。
type Daemon struct {
	cfg       config.Config
	server    *grpc.Server
	hub       *WatcherHub
	mhub      *MultiHostHub // 聚合所有主機快照（永遠初始化）
	hubMgr    *HubManager   // 管理多主機連線（永遠初始化）
	state     *StateManager
	store     *store.Store
	cancelRun context.CancelFunc
	HubDialFn HubDialFn // 外部注入的遠端 dial 函式（避免循環依賴）
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

	// 防護：socket 或 PID 任一存活則拒絕啟動
	if err := guardAlreadyRunning(sockPath, pidPath); err != nil {
		return err
	}

	// socket 不可用（殘留或不存在），安全清理
	os.Remove(sockPath)
	os.Remove(VersionPath(d.cfg))

	// 寫入 PID 檔案
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	// 寫入版本檔案
	verPath := VersionPath(d.cfg)
	if err := os.WriteFile(verPath, []byte(version.String()), 0o600); err != nil {
		return fmt.Errorf("write version file: %w", err)
	}
	defer os.Remove(verPath)

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

	// Hub 模式：永遠啟動 HubManager，統一由 Hub 管理所有主機快照聚合。
	// 即使只有 local，也透過 Hub 提供 WatchMultiHost stream。
	d.mhub = NewMultiHostHub()
	d.hubMgr = NewHubManager(d.mhub)
	d.hubMgr.dialFn = d.HubDialFn // 注入遠端 dial 函式
	for _, h := range d.cfg.Hosts {
		d.hubMgr.AddHost(h)
	}
	// 將 local WatcherHub 接入 hub
	for _, h := range d.cfg.Hosts {
		if h.IsLocal() && h.Enabled {
			d.hubMgr.attachLocal(d.hub, d.state, h)
			break
		}
	}

	svc := NewService(mgr, st, d.hub, d.mhub, d.hubMgr, d.state)

	// 設定 localMutationFn：讓 ProxyMutation 能透過 Service 操作本機 session
	if d.hubMgr != nil {
		d.hubMgr.localMutationFn = func(req *tsmv1.ProxyMutationRequest) error {
			ctx := context.Background()
			switch req.Type {
			case tsmv1.MutationType_MUTATION_KILL_SESSION:
				_, err := svc.KillSession(ctx, &tsmv1.KillSessionRequest{Name: req.SessionName})
				return err
			case tsmv1.MutationType_MUTATION_RENAME_SESSION:
				_, err := svc.RenameSession(ctx, &tsmv1.RenameSessionRequest{
					SessionName: req.SessionName,
					CustomName:  req.NewName,
				})
				return err
			case tsmv1.MutationType_MUTATION_CREATE_SESSION:
				_, err := svc.CreateSession(ctx, &tsmv1.CreateSessionRequest{
					Name: req.SessionName, Path: req.Path, Command: req.Command,
				})
				return err
			case tsmv1.MutationType_MUTATION_MOVE_SESSION:
				gid, _ := strconv.ParseInt(req.GroupId, 10, 64)
				_, err := svc.MoveSession(ctx, &tsmv1.MoveSessionRequest{
					SessionName: req.SessionName,
					GroupId:     gid,
				})
				return err
			default:
				return fmt.Errorf("unsupported local mutation type: %v", req.Type)
			}
		}
	}

	d.server = grpc.NewServer()
	tsmv1.RegisterSessionManagerServer(d.server, svc)

	// 啟動狀態掃描迴圈
	ctx, cancel := context.WithCancel(context.Background())
	d.cancelRun = cancel
	go d.state.Run(ctx)

	// 啟動遠端主機連線（hub 模式）
	if d.hubMgr != nil {
		d.hubMgr.StartRemoteHosts(ctx)
	}

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
	if d.hubMgr != nil {
		d.hubMgr.Close()
	}
	if d.mhub != nil {
		d.mhub.Close()
	}
	// 先關閉 hub，讓所有 Watch stream 結束，避免 GracefulStop 死鎖
	if d.hub != nil {
		d.hub.Close()
	}
	if d.server != nil {
		d.server.GracefulStop()
	}
}

// ResolveExe 回傳 daemon 應使用的 binary 路徑。
// 優先使用 PATH 中的 tsm（已安裝的版本），避免從暫存升級檔案啟動 daemon。
// 找不到時降級為 os.Executable()（開發環境）。
func ResolveExe() (string, error) {
	if p, err := exec.LookPath("tsm"); err == nil {
		return p, nil
	}
	return os.Executable()
}

// Start 以 re-exec 模式在背景啟動 daemon。
func Start(cfg config.Config) error {
	sockPath := SocketPath(cfg)
	pidPath := PidPath(cfg)
	if err := guardAlreadyRunning(sockPath, pidPath); err != nil {
		return err
	}

	exe, err := ResolveExe()
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

// Stop 停止由 PID 檔案指定的 daemon，等待程序退出。
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

	if !processExists(pid) {
		os.Remove(pidPath)
		fmt.Fprintf(os.Stderr, "daemon (pid=%d) already exited, cleaned pid file\n", pid)
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		os.Remove(pidPath)
		return fmt.Errorf("send SIGTERM to pid %d: %w", pid, err)
	}

	// 等待程序退出（最多 5 秒），每秒顯示倒數
	const totalSec = 5
	for sec := totalSec; sec > 0; sec-- {
		fmt.Fprintf(os.Stderr, "\r服務關閉中...%d ", sec)
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			if !processExists(pid) {
				os.Remove(pidPath)
				fmt.Fprintf(os.Stderr, "\rdaemon stopped (pid=%d)   \n", pid)
				return nil
			}
		}
	}

	// 超時後再確認一次
	os.Remove(pidPath)
	if !processExists(pid) {
		fmt.Fprintf(os.Stderr, "\rdaemon stopped (pid=%d)   \n", pid)
		return nil
	}
	return fmt.Errorf("daemon (pid=%d) did not exit within %ds after SIGTERM", pid, totalSec)
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

	// 讀取版本檔案（舊版 daemon 可能沒有）
	verPath := VersionPath(cfg)
	ver := ""
	if vdata, verErr := os.ReadFile(verPath); verErr == nil {
		ver = strings.TrimSpace(string(vdata))
	}

	if ver != "" {
		return fmt.Sprintf("daemon running (pid=%d, version=%s, socket=%s)", pid, ver, sockPath), nil
	}
	return fmt.Sprintf("daemon running (pid=%d, socket=%s)", pid, sockPath), nil
}

// IsRunning 檢查 daemon 是否正在執行。
func IsRunning(cfg config.Config) bool {
	return isRunning(PidPath(cfg))
}

// socketAlive 嘗試連線到 unix socket，判斷是否有 daemon 正在服務。
func socketAlive(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// guardAlreadyRunning 檢查是否已有 daemon 在執行（socket 或 PID）。
func guardAlreadyRunning(sockPath, pidPath string) error {
	if socketAlive(sockPath) {
		return fmt.Errorf("daemon already running (socket active: %s)", sockPath)
	}
	if isRunning(pidPath) {
		return fmt.Errorf("daemon already running (pid file: %s)", pidPath)
	}
	return nil
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
