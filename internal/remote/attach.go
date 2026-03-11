package remote

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// sshRunFn 執行 SSH 命令，可在測試中替換。
var sshRunFn = func(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// AttachResult 代表 SSH attach 工作階段的結果。
type AttachResult int

const (
	// AttachDetached 表示使用者正常 detach（exit 0）。
	AttachDetached AttachResult = iota
	// AttachDisconnected 表示連線中斷（exit != 0）。
	AttachDisconnected
)

// classifyExit 將結束代碼對應到 AttachResult。
func classifyExit(exitCode int) AttachResult {
	if exitCode == 0 {
		return AttachDetached
	}
	return AttachDisconnected
}

// attachArgs 建構 SSH attach 的命令列參數，含 keepalive 偵測斷線。
func attachArgs(host, sessionName string) []string {
	remoteCmd := fmt.Sprintf("exec $SHELL -lc 'tmux attach-session -t %q'", sessionName)
	return []string{
		"-t",
		"-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=3",
		host, remoteCmd,
	}
}

// flushStdin 清空終端輸入緩衝區中殘留的 escape sequence 回應。
// 重連時 Bubble Tea alt screen 退出後，local tmux 可能重新查詢終端，
// iTerm2 的 XTVERSION DCS 回應會殘留在 stdin，若不清除會洩漏到遠端 shell。
func flushStdin() {
	time.Sleep(50 * time.Millisecond)
	fd := int(os.Stdin.Fd())
	if err := syscall.SetNonblock(fd, true); err != nil {
		return
	}
	buf := make([]byte, 256)
	for {
		n, _ := syscall.Read(fd, buf)
		if n <= 0 {
			break
		}
	}
	_ = syscall.SetNonblock(fd, false)
}

// SetHubSocket 在遠端主機的 tmux 設定 hub socket 路徑。
func SetHubSocket(host, socketPath string) error {
	return sshRunFn("ssh", host, "tmux", "set-option", "-g", "@tsm_hub_socket", socketPath)
}

// ClearHubSocket 清除遠端主機的 hub socket 設定。
func ClearHubSocket(host string) error {
	return sshRunFn("ssh", host, "tmux", "set-option", "-gu", "@tsm_hub_socket")
}

// Attach 啟動 `ssh -t host` 並透過遠端 login shell 執行 tmux attach-session。
// 使用 login shell 確保 PATH 包含 Homebrew 等非系統路徑。
// 使用者正常 detach 回傳 AttachDetached，連線中斷回傳 AttachDisconnected。
func Attach(host, sessionName string) AttachResult {
	flushStdin()

	args := attachArgs(host, sessionName)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err == nil {
		return classifyExit(0)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return classifyExit(exitErr.ExitCode())
	}
	return AttachDisconnected
}
