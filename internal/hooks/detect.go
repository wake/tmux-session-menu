package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wake/tmux-session-menu/internal/setup"
)

// HooksStatus 表示 Claude Code hooks 的偵測狀態。
type HooksStatus int

const (
	HooksNotInstalled HooksStatus = iota
	HooksPartial
	HooksInstalled
	HooksNoSettings
)

// HooksDetectResult 包含偵測結果的詳細資訊。
type HooksDetectResult struct {
	Status          HooksStatus
	InstalledEvents []string
	MissingEvents   []string
}

// 可替換的函式，方便測試 BuildComponent。
var (
	settingsPathFn = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".claude", "settings.json")
	}
	scriptPathFn = func() string {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".config", "tsm", "hooks", "tsm-hook.sh")
	}
)

// Detect 偵測 Claude Code hooks 的安裝狀態。
func (m *Manager) Detect() HooksDetectResult {
	settings, err := m.readSettings()
	if err != nil {
		return HooksDetectResult{Status: HooksNoSettings}
	}

	hooksMap := getOrCreateHooksMap(settings)

	var installed, missing []string
	for _, event := range tsmEvents {
		var entries []interface{}
		if raw, ok := hooksMap[event]; ok {
			if arr, ok := raw.([]interface{}); ok {
				entries = arr
			}
		}
		if m.hasTSMHook(entries) {
			installed = append(installed, event)
		} else {
			missing = append(missing, event)
		}
	}

	total := len(tsmEvents)
	switch len(installed) {
	case total:
		return HooksDetectResult{
			Status:          HooksInstalled,
			InstalledEvents: installed,
		}
	case 0:
		return HooksDetectResult{
			Status:        HooksNotInstalled,
			MissingEvents: missing,
		}
	default:
		return HooksDetectResult{
			Status:          HooksPartial,
			InstalledEvents: installed,
			MissingEvents:   missing,
		}
	}
}

// BuildComponent 根據偵測結果建立 setup.Component。
func BuildComponent() setup.Component {
	settingsPath := settingsPathFn()
	scriptPath := scriptPathFn()

	if settingsPath == "" || scriptPath == "" {
		return setup.Component{
			Label:    "Claude Code hooks",
			Checked:  false,
			Disabled: true,
			Note:     "無法取得 home 目錄",
			InstallFn: func() (string, error) {
				return "", fmt.Errorf("無法取得 home 目錄")
			},
			UninstallFn: func() (string, error) {
				return "", fmt.Errorf("無法取得 home 目錄")
			},
		}
	}

	mgr := NewManager(settingsPath, scriptPath)
	r := mgr.Detect()

	total := len(tsmEvents)

	installFn := func() (string, error) {
		m := NewManager(settingsPath, scriptPath)
		if err := m.EnsureScript(); err != nil {
			return "", err
		}
		result, err := m.Install(false)
		if err != nil {
			return "", err
		}
		if !result.Changed {
			return "已安裝，無需變更", nil
		}
		return fmt.Sprintf("已新增 hooks: %s", joinStrings(result.EventsAdded)), nil
	}
	uninstallFn := func() (string, error) {
		m := NewManager(settingsPath, scriptPath)
		result, err := m.Uninstall(false)
		if err != nil {
			return "", err
		}
		if !result.Changed {
			return "未偵測到 hooks，無需移除", nil
		}
		return fmt.Sprintf("已移除 hooks: %s", joinStrings(result.EventsRemoved)), nil
	}

	switch r.Status {
	case HooksInstalled:
		return setup.Component{
			Label:       "Claude Code hooks",
			Checked:     false,
			Installed:   true,
			Note:        fmt.Sprintf("已安裝（%d/%d events）", total, total),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	case HooksPartial:
		return setup.Component{
			Label:       "Claude Code hooks",
			Checked:     true,
			Installed:   true,
			Note:        fmt.Sprintf("部分安裝（%d/%d events）", len(r.InstalledEvents), total),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	default: // HooksNotInstalled, HooksNoSettings
		return setup.Component{
			Label:       "Claude Code hooks",
			Checked:     true,
			Note:        fmt.Sprintf("將寫入 %s", settingsPath),
			InstallFn:   installFn,
			UninstallFn: uninstallFn,
		}
	}
}

// joinStrings 用逗號串聯字串切片。
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
