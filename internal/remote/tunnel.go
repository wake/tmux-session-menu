// Package remote 提供遠端 tsm 連線所需的通道與 SSH 工具函式。
package remote

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LocalSocketPath 根據主機名稱產生確定性的本地 Unix socket 路徑。
func LocalSocketPath(host string) string {
	h := sha256.Sum256([]byte(host))
	return filepath.Join(os.TempDir(), fmt.Sprintf("tsm-%x.sock", h[:8]))
}

// ReverseSocketPath 根據主機名稱產生確定性的 hub reverse tunnel 遠端 socket 路徑。
// 使用 "hub:" 前綴以確保與 LocalSocketPath 不同。
// 放在 ~/.config/tsm/ 下避免 macOS tmp_cleaner 每日清理 /tmp 中超過 3 天的 socket。
func ReverseSocketPath(host string) string {
	h := sha256.Sum256([]byte("hub:" + host))
	home, err := os.UserHomeDir()
	if err != nil {
		// fallback to /tmp
		return filepath.Join("/tmp", fmt.Sprintf("tsm-hub-%x.sock", h[:8]))
	}
	dir := filepath.Join(home, ".config", "tsm")
	os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, fmt.Sprintf("tsm-hub-%x.sock", h[:8]))
}

// tunnelArgs 建構 SSH 通道轉發所需的命令列參數，含 keepalive 偵測斷線。
func tunnelArgs(host, localSock, remoteSock string) []string {
	fwd := fmt.Sprintf("%s:%s", localSock, remoteSock)
	return []string{
		"ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
		"-L", fwd, host,
	}
}

// tunnelArgsWithReverse 建構同時含 -L 與 -R 的 SSH 命令列參數。
// reverseRemoteSock 為遠端主機上的 socket 路徑，reverseLocalSock 為本地 hub daemon socket 路徑。
func tunnelArgsWithReverse(host, localSock, remoteSock, reverseRemoteSock, reverseLocalSock string) []string {
	fwdL := fmt.Sprintf("%s:%s", localSock, remoteSock)
	fwdR := fmt.Sprintf("%s:%s", reverseRemoteSock, reverseLocalSock)
	return []string{
		"ssh", "-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
		"-L", fwdL,
		"-R", fwdR,
		host,
	}
}

// CmdResult 封裝命令執行的輸出與錯誤。
type CmdResult struct {
	Output string
	Err    error
}

// CmdRunFunc 執行命令並回傳結果（用於依賴注入）。
type CmdRunFunc func(name string, args ...string) CmdResult

// Process 抽象化執行中的程序，供清理時使用。
type Process interface {
	Kill() error
	Wait() error
}

// CmdStartFunc 啟動命令程序並回傳可控制的 Process（用於依賴注入）。
type CmdStartFunc func(name string, args ...string) (Process, error)

// Tunnel 管理 SSH Unix socket 轉發通道。
type Tunnel struct {
	host      string
	localSock string
	proc      Process
	cmdRun    CmdRunFunc
	cmdStart  CmdStartFunc

	hasReverse        bool   // 是否啟用 reverse tunnel (-R)
	reverseLocalSock  string // 本地 hub daemon socket 路徑
	reverseRemoteSock string // 遠端主機的 reverse socket 路徑
}

// TunnelOption 用於設定 Tunnel 的選項函式。
type TunnelOption func(*Tunnel)

// WithCmdRun 注入自訂的命令執行函式。
func WithCmdRun(fn CmdRunFunc) TunnelOption {
	return func(t *Tunnel) { t.cmdRun = fn }
}

// WithCmdStart 注入自訂的命令啟動函式。
func WithCmdStart(fn CmdStartFunc) TunnelOption {
	return func(t *Tunnel) { t.cmdStart = fn }
}

// WithReverse 啟用 SSH reverse tunnel，將遠端主機的 hub socket 轉發到本地 hubSocketPath。
func WithReverse(hubSocketPath string) TunnelOption {
	return func(t *Tunnel) {
		t.hasReverse = true
		t.reverseLocalSock = hubSocketPath
		t.reverseRemoteSock = ReverseSocketPath(t.host)
	}
}

// defaultCmdRun 使用 os/exec 執行命令並回傳輸出。
func defaultCmdRun(name string, args ...string) CmdResult {
	out, err := exec.Command(name, args...).Output()
	return CmdResult{Output: string(out), Err: err}
}

// osProcess 將 os.Process 包裝為符合 Process 介面的型別。
type osProcess struct {
	p *os.Process
}

func (o *osProcess) Kill() error {
	return o.p.Kill()
}

func (o *osProcess) Wait() error {
	_, err := o.p.Wait()
	return err
}

// defaultCmdStart 使用 os/exec 啟動背景命令。
func defaultCmdStart(name string, args ...string) (Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osProcess{p: cmd.Process}, nil
}

// NewTunnel 建立新的 Tunnel 實例，可透過 TunnelOption 自訂行為。
func NewTunnel(host string, opts ...TunnelOption) *Tunnel {
	t := &Tunnel{
		host:      host,
		localSock: LocalSocketPath(host),
		cmdRun:    defaultCmdRun,
		cmdStart:  defaultCmdStart,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Host 回傳遠端主機名稱。
func (t *Tunnel) Host() string { return t.host }

// LocalSocket 回傳本地 Unix socket 路徑。
func (t *Tunnel) LocalSocket() string { return t.localSock }

// Start 解析遠端 socket 路徑、建立 SSH 通道並等待連線就緒。
func (t *Tunnel) Start() error {
	remoteSock, err := t.resolveRemoteSocket()
	if err != nil {
		return fmt.Errorf("resolve remote socket: %w", err)
	}

	os.Remove(t.localSock)

	// 清除遠端 stale reverse socket，避免 sshd bind 失敗導致 ExitOnForwardFailure 殺掉整條 tunnel
	if t.hasReverse {
		t.cmdRun("ssh", t.host, "rm", "-f", t.reverseRemoteSock)
	}

	var args []string
	if t.hasReverse {
		args = tunnelArgsWithReverse(t.host, t.localSock, remoteSock, t.reverseRemoteSock, t.reverseLocalSock)
	} else {
		args = tunnelArgs(t.host, t.localSock, remoteSock)
	}
	proc, err := t.cmdStart(args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("start ssh tunnel: %w", err)
	}
	t.proc = proc

	if err := waitForSocket(t.localSock, 5*time.Second); err != nil {
		t.Close()
		return err
	}
	return nil
}

// Close 終止 SSH 程序並移除本地 socket 檔案。
func (t *Tunnel) Close() {
	if t.proc != nil {
		t.proc.Kill()
		t.proc.Wait()
		t.proc = nil
	}
	os.Remove(t.localSock)
}

// resolveRemoteSocket 透過 SSH 取得遠端主機的 tsm socket 絕對路徑。
func (t *Tunnel) resolveRemoteSocket() (string, error) {
	result := t.cmdRun("ssh", t.host, "echo", "$HOME/.config/tsm/tsm.sock")
	if result.Err != nil {
		return "", fmt.Errorf("ssh to %s: %w", t.host, result.Err)
	}
	path := strings.TrimSpace(result.Output)
	if path == "" {
		return "", fmt.Errorf("empty remote socket path from %s", t.host)
	}
	return path, nil
}

// waitForSocket 輪詢等待 Unix socket 可連線，逾時回傳錯誤。
func waitForSocket(sockPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("tunnel socket %s not ready within %s", sockPath, timeout)
}
