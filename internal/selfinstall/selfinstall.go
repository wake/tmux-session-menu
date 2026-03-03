package selfinstall

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wake/tmux-session-menu/internal/setup"
	"github.com/wake/tmux-session-menu/internal/version"
)

// Status 表示全域 tsm 指令的偵測狀態。
type Status int

const (
	StatusNotFound    Status = iota // 未安裝
	StatusConflict                  // 路徑上有同名但非 tsm 的指令
	StatusOutdated                  // 版本不同
	StatusSameVersion               // 版本相同
)

// DetectResult 包含偵測結果的詳細資訊。
type DetectResult struct {
	Status  Status
	Path    string // 已安裝的路徑（StatusNotFound 時為空）
	Version string // 已安裝的版本（StatusConflict/StatusNotFound 時為空）
}

// 可替換的函式，方便測試。
var (
	lookPathFn   = exec.LookPath
	runVersionFn = func(path string) (string, error) {
		out, err := exec.Command(path, "--version").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	goPathBinFn = func() string {
		if gopath := os.Getenv("GOPATH"); gopath != "" {
			return filepath.Join(gopath, "bin")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, "go", "bin")
	}
)

// Detect 偵測全域 tsm 指令的狀態。
func Detect() DetectResult {
	path, err := lookPathFn("tsm")
	if err != nil {
		return DetectResult{Status: StatusNotFound}
	}

	output, err := runVersionFn(path)
	if err != nil || !strings.HasPrefix(output, "tsm ") {
		return DetectResult{Status: StatusConflict, Path: path}
	}

	installedVersion := strings.TrimPrefix(output, "tsm ")
	currentVersion := version.String()

	if installedVersion != currentVersion {
		return DetectResult{Status: StatusOutdated, Path: path, Version: installedVersion}
	}

	return DetectResult{Status: StatusSameVersion, Path: path, Version: installedVersion}
}

// Install 將目前執行檔複製到 targetDir/tsm。
// 使用 .tmp 暫存檔 + rename 確保原子性。
func Install(targetDir string) (string, error) {
	srcExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("無法取得執行檔路徑: %w", err)
	}
	srcExe, err = filepath.EvalSymlinks(srcExe)
	if err != nil {
		return "", fmt.Errorf("無法解析 symlink: %w", err)
	}

	target := filepath.Join(targetDir, "tsm")

	// 避免自我覆蓋
	if realTarget, err := filepath.EvalSymlinks(target); err == nil {
		if realTarget == srcExe {
			return fmt.Sprintf("來源與目標相同，跳過: %s", target), nil
		}
	}

	// 確保目標目錄存在
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("無法建立目錄 %s: %w", targetDir, err)
	}

	// 複製到 .tmp 再 rename
	tmpPath := target + ".tmp"
	if err := copyFile(srcExe, tmpPath); err != nil {
		return "", fmt.Errorf("複製失敗: %w", err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("安裝失敗: %w", err)
	}

	return fmt.Sprintf("已安裝到 %s", target), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// BuildComponent 根據偵測結果建立 setup.Component。
func BuildComponent() setup.Component {
	r := Detect()

	switch r.Status {
	case StatusNotFound:
		targetDir := goPathBinFn()
		if targetDir == "" {
			return setup.Component{
				Label:    "安裝 tsm (無法偵測路徑)",
				Checked:  false,
				Disabled: true,
				Note:     "無法取得 $GOPATH/bin 路徑，請設定 GOPATH 環境變數",
				InstallFn: func() (string, error) {
					return "", fmt.Errorf("無法偵測安裝路徑")
				},
				UninstallFn: func() (string, error) {
					return "", fmt.Errorf("尚未安裝")
				},
			}
		}
		return setup.Component{
			Label:   fmt.Sprintf("安裝 tsm 到 %s", filepath.Join(targetDir, "tsm")),
			Checked: true,
			InstallFn: func() (string, error) {
				return Install(targetDir)
			},
			UninstallFn: func() (string, error) {
				return "", fmt.Errorf("尚未安裝")
			},
		}

	case StatusConflict:
		return setup.Component{
			Label:    "安裝 tsm (衝突)",
			Checked:  false,
			Disabled: true,
			Note:     fmt.Sprintf("路徑 %s 已存在非 tsm 的同名指令", r.Path),
			InstallFn: func() (string, error) {
				return "", fmt.Errorf("路徑衝突")
			},
			UninstallFn: func() (string, error) {
				return "", fmt.Errorf("路徑衝突")
			},
		}

	case StatusOutdated:
		targetDir := filepath.Dir(r.Path)
		return setup.Component{
			Label:   fmt.Sprintf("更新 tsm 到 %s", r.Path),
			Checked: true,
			Note:    fmt.Sprintf("目前版本: %s → %s", r.Version, version.String()),
			InstallFn: func() (string, error) {
				return Install(targetDir)
			},
			UninstallFn: func() (string, error) {
				if err := os.Remove(r.Path); err != nil {
					return "", err
				}
				return fmt.Sprintf("已移除 %s", r.Path), nil
			},
		}

	default: // StatusSameVersion
		targetDir := filepath.Dir(r.Path)
		return setup.Component{
			Label:   fmt.Sprintf("覆蓋安裝 tsm 到 %s", r.Path),
			Checked: false,
			Note:    fmt.Sprintf("已安裝版本: %s", r.Version),
			InstallFn: func() (string, error) {
				return Install(targetDir)
			},
			UninstallFn: func() (string, error) {
				if err := os.Remove(r.Path); err != nil {
					return "", err
				}
				return fmt.Sprintf("已移除 %s", r.Path), nil
			},
		}
	}
}
