package bind

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	"github.com/wake/tmux-session-menu/internal/setup"
)

const (
	markerBegin = "# [tsm] begin"
	markerEnd   = "# [tsm] end"
	bindBlock   = `# [tsm] begin
bind-key -n C-q display-popup -E -w 80% -h 80% "tsm --inline"
# [tsm] end`
)

// BindStatus 表示 tmux keybinding 的偵測狀態。
type BindStatus int

const (
	BindNotInstalled BindStatus = iota
	BindInstalled
	BindNoConfFile
)

// BindDetectResult 包含偵測結果的詳細資訊。
type BindDetectResult struct {
	Status   BindStatus
	ConfPath string
}

// Detect 偵測 tmux keybinding 的安裝狀態。
func Detect() BindDetectResult {
	confPath, err := tmuxConfPath()
	if err != nil {
		return BindDetectResult{Status: BindNoConfFile}
	}

	content, _ := readFileOrEmpty(confPath)
	if strings.Contains(content, markerBegin) {
		return BindDetectResult{Status: BindInstalled, ConfPath: confPath}
	}
	return BindDetectResult{Status: BindNotInstalled, ConfPath: confPath}
}

// BuildComponent 根據偵測結果建立 setup.Component。
func BuildComponent() setup.Component {
	r := Detect()

	installFn := func() (string, error) {
		result, err := Install(false)
		if err != nil {
			return "", err
		}
		return result.Message, nil
	}
	uninstallFn := func() (string, error) {
		result, err := Uninstall(false)
		if err != nil {
			return "", err
		}
		return result.Message, nil
	}

	switch r.Status {
	case BindInstalled:
		return setup.Component{
			Label:       "Ctrl+Q 快捷鍵 (tmux keybinding)",
			Checked:     false,
			Installed:   true,
			Note:        fmt.Sprintf("已安裝於 %s", r.ConfPath),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	case BindNoConfFile:
		return setup.Component{
			Label:    "Ctrl+Q 快捷鍵 (tmux keybinding)",
			Checked:  false,
			Disabled: true,
			Note:     "無法取得 tmux.conf 路徑",
			InstallFn: func() (string, error) {
				return "", fmt.Errorf("無法取得 tmux.conf 路徑")
			},
			UninstallFn: func() (string, error) {
				return "", fmt.Errorf("無法取得 tmux.conf 路徑")
			},
		}
	default: // BindNotInstalled
		return setup.Component{
			Label:       "Ctrl+Q 快捷鍵 (tmux keybinding)",
			Checked:     true,
			Note:        fmt.Sprintf("將寫入 %s", r.ConfPath),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	}
}

// Result 代表 install/uninstall 的操作結果。
type Result struct {
	Changed  bool
	FilePath string
	Message  string
}

// Install 將 Ctrl+Q 快捷鍵加入 tmux.conf。
func Install(dryRun bool) (*Result, error) {
	confPath, err := tmuxConfPath()
	if err != nil {
		return nil, err
	}

	content, err := readFileOrEmpty(confPath)
	if err != nil {
		return nil, fmt.Errorf("讀取 %s: %w", confPath, err)
	}

	if strings.Contains(content, markerBegin) {
		// 檢查是否需要升級（block 內容是否與最新版一致）
		if strings.Contains(content, bindBlock) {
			return &Result{Changed: false, FilePath: confPath, Message: "Ctrl+Q 快捷鍵已存在"}, nil
		}
		// 移除舊 block，後續會重新寫入最新版
		content = removeBlock(content)
	}

	newContent := content
	if newContent != "" && !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += bindBlock + "\n"

	if dryRun {
		return &Result{Changed: true, FilePath: confPath, Message: fmt.Sprintf("將寫入 %s", confPath)}, nil
	}

	if err := os.WriteFile(confPath, []byte(newContent), 0o644); err != nil {
		return nil, fmt.Errorf("寫入 %s: %w", confPath, err)
	}

	reloadTmuxConf(confPath)

	return &Result{Changed: true, FilePath: confPath, Message: fmt.Sprintf("已寫入 %s", confPath)}, nil
}

// Uninstall 從 tmux.conf 移除 Ctrl+Q 快捷鍵。
func Uninstall(dryRun bool) (*Result, error) {
	confPath, err := tmuxConfPath()
	if err != nil {
		return nil, err
	}

	content, err := readFileOrEmpty(confPath)
	if err != nil {
		return nil, fmt.Errorf("讀取 %s: %w", confPath, err)
	}

	if !strings.Contains(content, markerBegin) {
		return &Result{Changed: false, FilePath: confPath, Message: "未偵測到 tsm 快捷鍵"}, nil
	}

	newContent := removeBlock(content)

	if dryRun {
		return &Result{Changed: true, FilePath: confPath, Message: fmt.Sprintf("將從 %s 移除 tsm 快捷鍵", confPath)}, nil
	}

	if err := os.WriteFile(confPath, []byte(newContent), 0o644); err != nil {
		return nil, fmt.Errorf("寫入 %s: %w", confPath, err)
	}

	reloadTmuxConf(confPath)

	return &Result{Changed: true, FilePath: confPath, Message: fmt.Sprintf("已從 %s 移除 tsm 快捷鍵", confPath)}, nil
}

// removeBlock 移除 marker 之間的區塊（含 marker 行）。
func removeBlock(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false
	for _, line := range lines {
		if strings.TrimSpace(line) == markerBegin {
			inBlock = true
			continue
		}
		if strings.TrimSpace(line) == markerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}

	out := strings.Join(result, "\n")
	// 清理尾部多餘空行
	out = strings.TrimRight(out, "\n") + "\n"
	return out
}

// tmuxConfPathFn 可在測試中替換。
var tmuxConfPathFn = defaultTmuxConfPath

func tmuxConfPath() (string, error) {
	return tmuxConfPathFn()
}

func defaultTmuxConfPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("無法取得 home 目錄: %w", err)
	}
	return filepath.Join(home, ".tmux.conf"), nil
}

func readFileOrEmpty(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// reloadTmuxConf 嘗試重新載入 tmux 設定（若 tmux 正在執行）。
func reloadTmuxConf(confPath string) {
	_ = osexec.Command("tmux", "source-file", confPath).Run()
}
