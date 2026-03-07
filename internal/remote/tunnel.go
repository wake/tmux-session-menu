// Package remote 提供遠端 tsm 連線所需的通道與 SSH 工具函式。
package remote

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// LocalSocketPath 根據主機名稱產生確定性的本地 Unix socket 路徑。
func LocalSocketPath(host string) string {
	h := sha256.Sum256([]byte(host))
	return filepath.Join(os.TempDir(), fmt.Sprintf("tsm-%x.sock", h[:8]))
}

// tunnelArgs 建構 SSH 通道轉發所需的命令列參數。
func tunnelArgs(host, localSock, remoteSock string) []string {
	fwd := fmt.Sprintf("%s:%s", localSock, remoteSock)
	return []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes", "-L", fwd, host}
}
