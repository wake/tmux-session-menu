package main

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wake/tmux-session-menu/internal/bind"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/daemon"
	"github.com/wake/tmux-session-menu/internal/hooks"
	"github.com/wake/tmux-session-menu/internal/selfinstall"
	"github.com/wake/tmux-session-menu/internal/setup"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
	"github.com/wake/tmux-session-menu/internal/version"

)

// runMode 表示 TUI 的啟動模式
type runMode int

const (
	modeAuto   runMode = iota // 自動偵測：在 tmux 內使用 popup，否則使用 inline
	modeInline                // 強制使用內嵌全螢幕模式
	modePopup                 // 強制使用 tmux popup 模式
)

// parseRunMode 從命令列參數中解析啟動模式
func parseRunMode(args []string) runMode {
	for _, a := range args {
		switch a {
		case "--inline":
			return modeInline
		case "--popup":
			return modePopup
		}
	}
	return modeAuto
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runWithMode(modeAuto)
		return
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("tsm %s\n", version.String())
		return
	}

	if args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return // exit 0
	}

	if args[0] == "--inline" || args[0] == "--popup" {
		runWithMode(parseRunMode(args))
		return
	}

	if args[0] == "bind" {
		runBind(args[1:])
		return
	}

	if args[0] == "hooks" {
		runHooks(args[1:])
		return
	}

	if args[0] == "daemon" {
		runDaemon(args[1:])
		return
	}

	if args[0] == "setup" {
		runSetup(args[1:])
		return
	}

	printUsage()
	os.Exit(1)
}

// runWithMode 根據指定模式啟動 TUI
func runWithMode(mode runMode) {
	// 自動偵測：若在 tmux 內，使用 popup 模式
	if mode == modeAuto && os.Getenv("TMUX") != "" {
		mode = modePopup
	}

	if mode == modePopup {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cmd := osexec.Command("tmux", "display-popup", "-E", "-w", "80%", "-h", "80%", exe, "--inline")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Popup 被使用者關閉（正常退出）
			os.Exit(0)
		}
		return
	}

	runTUI()
}

func loadConfig() config.Config {
	cfg := config.Default()
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			cfg = loaded
		}
	}
	return cfg
}

func runTUI() {
	cfg := loadConfig()

	// 嘗試連線 daemon（會自動啟動）
	c, err := client.Dial(cfg)
	if err == nil {
		defer c.Close()
		runTUIWithClient(c, cfg)
		return
	}

	// Daemon 連線失敗，降級到直接模式
	fmt.Fprintf(os.Stderr, "Warning: daemon 連線失敗，使用直接模式: %v\n", err)
	runTUILegacy(cfg)
}

// runTUIWithClient 使用 gRPC daemon 模式啟動 TUI。
func runTUIWithClient(c *client.Client, cfg config.Config) {
	deps := ui.Deps{
		Client: c,
		Cfg:    cfg,
	}

	m := ui.NewModel(deps)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(ui.Model); ok {
		if selected := m.Selected(); selected != "" {
			switchToSession(selected)
		}
	}
}

// runTUILegacy 使用舊的直接模式啟動 TUI（不透過 daemon）。
func runTUILegacy(cfg config.Config) {
	dataDir := config.ExpandPath(cfg.DataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: 無法建立資料目錄 %s: %v\n", dataDir, err)
	}

	var st *store.Store
	dbPath := filepath.Join(dataDir, "state.db")
	if s, err := store.Open(dbPath); err == nil {
		st = s
		defer st.Close()
	} else {
		fmt.Fprintf(os.Stderr, "Warning: 無法開啟資料庫 %s: %v\n", dbPath, err)
	}

	exec := tmux.NewRealExecutor()
	mgr := tmux.NewManager(exec)

	statusDir := filepath.Join(dataDir, "status")

	deps := ui.Deps{
		TmuxMgr:   mgr,
		Store:     st,
		Cfg:       cfg,
		StatusDir: statusDir,
	}

	m := ui.NewModel(deps)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if m, ok := finalModel.(ui.Model); ok {
		if selected := m.Selected(); selected != "" {
			switchToSession(selected)
		}
	}
}

func switchToSession(name string) {
	var err error
	if os.Getenv("TMUX") != "" {
		err = osexec.Command("tmux", "switch-client", "-t", name).Run()
	} else {
		cmd := osexec.Command("tmux", "attach-session", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法切換到 session %q: %v\n", name, err)
		os.Exit(1)
	}
}

func runHooks(args []string) {
	if len(args) == 0 {
		printHooksUsage()
		os.Exit(1)
		return
	}

	if args[0] == "--help" || args[0] == "-h" {
		printHooksUsage()
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

		if dryRun {
			fmt.Printf("(dry-run) 將寫入 hook script: %s\n", scriptPath)
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
	fmt.Fprintf(os.Stderr, "tsm %s\n", version.String())
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage: tsm [command] [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  (no args)          啟動 TUI 選單（自動管理 daemon）")
	fmt.Fprintln(os.Stderr, "  setup              互動式安裝所有元件")
	fmt.Fprintln(os.Stderr, "  bind install       安裝 Ctrl+Q 快捷鍵到 ~/.tmux.conf")
	fmt.Fprintln(os.Stderr, "  bind uninstall     移除 Ctrl+Q 快捷鍵")
	fmt.Fprintln(os.Stderr, "  daemon start       前景啟動 daemon")
	fmt.Fprintln(os.Stderr, "  daemon stop        停止 daemon")
	fmt.Fprintln(os.Stderr, "  daemon status      顯示 daemon 狀態")
	fmt.Fprintln(os.Stderr, "  hooks install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  hooks uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --version, -v      顯示版本號")
	fmt.Fprintln(os.Stderr, "  --inline           強制使用內嵌全螢幕模式")
	fmt.Fprintln(os.Stderr, "  --popup            強制使用 tmux popup 模式")
	fmt.Fprintln(os.Stderr, "  --dry-run          預覽變更，不實際寫入")
}

func runDaemon(args []string) {
	if len(args) == 0 {
		printDaemonUsage()
		os.Exit(1)
		return
	}

	if args[0] == "--help" || args[0] == "-h" {
		printDaemonUsage()
		return
	}

	cfg := loadConfig()

	switch args[0] {
	case "start":
		d := daemon.NewDaemon(cfg)
		if err := d.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "stop":
		if err := daemon.Stop(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "status":
		status, err := daemon.Status(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Println(status)
	default:
		printDaemonUsage()
		os.Exit(1)
	}
}

func printDaemonUsage() {
	fmt.Fprintln(os.Stderr, "Usage: tsm daemon <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  start      前景啟動 daemon")
	fmt.Fprintln(os.Stderr, "  stop       停止 daemon（送 SIGTERM）")
	fmt.Fprintln(os.Stderr, "  status     顯示 daemon 狀態")
}

func runBind(args []string) {
	if len(args) == 0 {
		printBindUsage()
		os.Exit(1)
		return
	}

	if args[0] == "--help" || args[0] == "-h" {
		printBindUsage()
		return
	}

	action := args[0]
	dryRun := containsFlag(args[1:], "--dry-run")

	switch action {
	case "install":
		result, err := bind.Install(dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !result.Changed {
			fmt.Println(result.Message)
			return
		}
		if dryRun {
			fmt.Printf("(dry-run) %s\n", result.Message)
		} else {
			fmt.Println(result.Message)
			fmt.Println("Ctrl+Q 快捷鍵已安裝，按 Ctrl+Q 即可開啟 tsm。")
		}

	case "uninstall":
		result, err := bind.Uninstall(dryRun)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !result.Changed {
			fmt.Println(result.Message)
			return
		}
		if dryRun {
			fmt.Printf("(dry-run) %s\n", result.Message)
		} else {
			fmt.Println(result.Message)
		}

	default:
		printBindUsage()
		os.Exit(1)
	}
}

func printBindUsage() {
	fmt.Fprintln(os.Stderr, "Usage: tsm bind <action> [--dry-run]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Actions:")
	fmt.Fprintln(os.Stderr, "  install      安裝 Ctrl+Q 快捷鍵到 ~/.tmux.conf")
	fmt.Fprintln(os.Stderr, "  uninstall    從 ~/.tmux.conf 移除 Ctrl+Q 快捷鍵")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --dry-run    預覽變更，不實際寫入")
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

func runSetup(args []string) {
	if containsFlag(args, "--help") || containsFlag(args, "-h") {
		fmt.Fprintln(os.Stderr, "Usage: tsm setup [--uninstall]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "互動式安裝所有 tsm 元件。")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fmt.Fprintln(os.Stderr, "  --uninstall  移除所有已安裝的元件")
		return
	}

	components := buildSetupComponents()

	if containsFlag(args, "--uninstall") {
		setup.RunUninstallAll(components)
		restartDaemon()
		return
	}

	m := setup.NewModel(components)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	restartDaemon()
}

// restartDaemon 嘗試重啟 daemon，確保使用最新 binary。
func restartDaemon() {
	cfg := loadConfig()
	// 嘗試停止（忽略錯誤，daemon 可能未運行）
	_ = daemon.Stop(cfg)
	// 等待舊程序釋放 socket
	time.Sleep(500 * time.Millisecond)
	// 用目前執行檔重新啟動 daemon（背景 fork）
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := osexec.Command(exe, "daemon", "start")
	_ = cmd.Start()
	go func() { _ = cmd.Wait() }()
}

func buildSetupComponents() []setup.Component {
	return []setup.Component{
		selfinstall.BuildComponent(),
		bind.BuildComponent(),
		hooks.BuildComponent(),
	}
}
