package tmux

import (
	"fmt"

	"github.com/wake/tmux-session-menu/internal/config"
)

// StatusBarArgs 根據顏色設定產生 tmux set-option 參數陣列。
// 回傳格式為 [][]string，每個子陣列是一組 tmux 指令參數。
// bar_bg 為空時不設 status-style（保持 tmux 預設）。
func StatusBarArgs(colors config.ColorConfig) [][]string {
	var cmds [][]string

	// 狀態列背景色（僅在有設定時才套用）
	if colors.BarBG != "" {
		cmds = append(cmds, []string{
			"set-option", "-g", "status-style",
			fmt.Sprintf("bg=%s", colors.BarBG),
		})
	}

	// 徽章樣式：背景色 + 前景色 + 粗體，內含動態名稱
	badgeStyle := fmt.Sprintf("#[bg=%s,fg=%s,bold]", colors.BadgeBG, colors.BadgeFG)
	resetStyle := "#[default]"
	statusLeft := fmt.Sprintf("%s #(tsm status-name) %s ", badgeStyle, resetStyle)

	cmds = append(cmds, []string{
		"set-option", "-g", "status-left", statusLeft,
	})
	cmds = append(cmds, []string{
		"set-option", "-g", "status-left-length", "30",
	})

	return cmds
}

// ApplyStatusBar 執行 tmux set-option 命令套用 status bar 樣式。
func ApplyStatusBar(exec Executor, colors config.ColorConfig) error {
	cmds := StatusBarArgs(colors)
	for _, cmd := range cmds {
		if _, err := exec.Execute(cmd...); err != nil {
			return fmt.Errorf("tmux %v: %w", cmd, err)
		}
	}
	return nil
}
