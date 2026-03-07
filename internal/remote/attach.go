package remote

import (
	"fmt"
	"os"
	"os/exec"
)

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

// Attach 啟動 `ssh -t host` 並透過遠端 login shell 執行 tmux attach-session。
// 使用 login shell 確保 PATH 包含 Homebrew 等非系統路徑。
// 使用者正常 detach 回傳 AttachDetached，連線中斷回傳 AttachDisconnected。
func Attach(host, sessionName string) AttachResult {
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
