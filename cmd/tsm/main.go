package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wake/tmux-session-menu/internal/hooks"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runTUI()
		return
	}

	if args[0] == "hooks" {
		runHooks(args[1:])
		return
	}

	printUsage()
	os.Exit(1)
}

func runTUI() {
	m := ui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runHooks(args []string) {
	if len(args) == 0 {
		printHooksUsage()
		os.Exit(1)
		return
	}

	action := args[0]
	dryRun := containsFlag(args[1:], "--dry-run")

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法取得 home 目錄: %v\n", err)
		os.Exit(1)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	scriptPath := filepath.Join(home, ".config", "tsm", "hooks", "tsm-hook.sh")
	mgr := hooks.NewManager(settingsPath, scriptPath)

	switch action {
	case "install":
		// 在非 dry-run 模式下，先確保 hook script 存在
		if !dryRun {
			if err := mgr.EnsureScript(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		result, err := mgr.Install(dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if !result.Changed {
			fmt.Println("tsm hooks 已安裝，無需變更。")
			return
		}

		if dryRun {
			fmt.Println("=== Dry Run ===")
			fmt.Printf("將新增 hooks 到以下 events: %s\n", strings.Join(result.EventsAdded, ", "))
			fmt.Println()
			fmt.Println(result.NewSettings)
		} else {
			fmt.Printf("已新增 hooks 到以下 events: %s\n", strings.Join(result.EventsAdded, ", "))
			fmt.Printf("已寫入 %s\n", settingsPath)
		}

	case "uninstall":
		result, err := mgr.Uninstall(dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if !result.Changed {
			fmt.Println("未偵測到 tsm hooks，無需移除。")
			return
		}

		if dryRun {
			fmt.Println("=== Dry Run ===")
			fmt.Printf("將移除 hooks 從以下 events: %s\n", strings.Join(result.EventsRemoved, ", "))
			fmt.Println()
			fmt.Println(result.NewSettings)
		} else {
			fmt.Printf("已移除 hooks 從以下 events: %s\n", strings.Join(result.EventsRemoved, ", "))
			fmt.Printf("已寫入 %s\n", settingsPath)
		}

	default:
		printHooksUsage()
		os.Exit(1)
	}
}

func containsFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: tsm [command]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  (no args)          啟動 TUI 選單")
	fmt.Fprintln(os.Stderr, "  hooks install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  hooks uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --dry-run          預覽變更，不實際寫入")
}

func printHooksUsage() {
	fmt.Fprintln(os.Stderr, "Usage: tsm hooks <action> [--dry-run]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Actions:")
	fmt.Fprintln(os.Stderr, "  install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --dry-run    預覽變更，不實際寫入")
}
