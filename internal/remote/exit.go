package remote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const exitMarkerName = "exit-requested"

// ExitMarkerPath 回傳本機 exit-requested sentinel 檔案的路徑。
func ExitMarkerPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "tsm", exitMarkerName)
}

// WriteExitMarker 寫入 exit-requested sentinel 檔案（供遠端偵測用）。
func WriteExitMarker() error {
	p := ExitMarkerPath()
	if p == "" {
		return nil
	}
	return os.WriteFile(p, nil, 0o644)
}

// RemoveExitMarker 移除本機的 exit-requested sentinel 檔案。
func RemoveExitMarker() {
	if p := ExitMarkerPath(); p != "" {
		os.Remove(p)
	}
}

// CheckAndClearExitRequested 透過 SSH 檢查遠端主機是否存在 exit-requested sentinel 檔案。
// 若存在則移除並回傳 true，否則回傳 false。
func CheckAndClearExitRequested(host string) bool {
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=3",
		host,
		`f="$HOME/.config/tsm/exit-requested"; test -f "$f" && rm -f "$f" && echo 1`,
	)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "1"
}
