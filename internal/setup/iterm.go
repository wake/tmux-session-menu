package setup

import (
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
)

// DetectIterm2 檢查是否安裝了 iTerm2。
func DetectIterm2() bool {
	if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
		return true
	}
	out, err := osexec.Command("defaults", "read", "com.googlecode.iterm2", "Default Bookmark Guid").CombinedOutput()
	return err == nil && len(out) > 0
}

// ItermCoprocessCommand 回傳 fileDropCoprocess 的設定值。
func ItermCoprocessCommand(tsmPath string) string {
	return fmt.Sprintf(`"%s" iterm-coprocess \(filenames)`, tsmPath)
}

// InstallItermCoprocess 設定 iTerm2 的 fileDropCoprocess。
func InstallItermCoprocess() (string, error) {
	// 優先用 PATH 中的 tsm（selfinstall 已安裝到正確位置），
	// 避免升級時從暫存檔執行 setup 導致路徑錯誤。
	tsmPath, err := osexec.LookPath("tsm")
	if err != nil {
		tsmPath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("取得 tsm 路徑失敗: %w", err)
		}
	}

	cmd := ItermCoprocessCommand(tsmPath)
	if err := osexec.Command("defaults", "write", "com.googlecode.iterm2",
		"fileDropCoprocess", "-string", cmd).Run(); err != nil {
		return "", fmt.Errorf("defaults write 失敗: %w", err)
	}

	return fmt.Sprintf("已設定 fileDropCoprocess\n  手動設定: iTerm2 Settings → Advanced → 搜尋 \"file drop\"\n  值: %s", cmd), nil
}

// UninstallItermCoprocess 移除 iTerm2 的 fileDropCoprocess 設定。
func UninstallItermCoprocess() (string, error) {
	if err := osexec.Command("defaults", "delete", "com.googlecode.iterm2",
		"fileDropCoprocess").Run(); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return "fileDropCoprocess 未設定，跳過", nil
		}
		return "", fmt.Errorf("defaults delete 失敗: %w", err)
	}
	return "已移除 fileDropCoprocess 設定", nil
}

// DetectItermCoprocessInstalled 檢查 fileDropCoprocess 是否已設定為 tsm。
func DetectItermCoprocessInstalled() bool {
	out, err := osexec.Command("defaults", "read", "com.googlecode.iterm2",
		"fileDropCoprocess").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "tsm iterm-coprocess")
}
