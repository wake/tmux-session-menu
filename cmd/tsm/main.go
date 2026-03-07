package main

import (
	"context"
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
	"github.com/wake/tmux-session-menu/internal/remote"
	"github.com/wake/tmux-session-menu/internal/selfinstall"
	"github.com/wake/tmux-session-menu/internal/setup"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
	"github.com/wake/tmux-session-menu/internal/upgrade"
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

// parseRemoteHost 從命令列參數中提取 --remote <host> 的主機名稱。
func parseRemoteHost(args []string) string {
	for i, a := range args {
		if a == "--remote" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
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

	if args[0] == "--remote" {
		if remoteHost := parseRemoteHost(args); remoteHost != "" {
			runRemote(remoteHost)
		} else {
			fmt.Fprintln(os.Stderr, "Error: --remote 需要指定主機名稱")
			fmt.Fprintln(os.Stderr, "Usage: tsm --remote <host>")
			os.Exit(1)
		}
		return
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

	if args[0] == "upgrade" {
		runUpgrade()
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

func runRemote(host string) {
	tun := remote.NewTunnel(host)
	if err := tun.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法建立 SSH tunnel 到 %s: %v\n", host, err)
		fmt.Fprintf(os.Stderr, "請確認遠端主機的 tsm-daemon 已啟動 (tsm daemon start)\n")
		os.Exit(1)
	}
	defer tun.Close()

	c, err := client.DialSocket(tun.LocalSocket())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 遠端 %s 的 tsm-daemon 未回應: %v\n", host, err)
		fmt.Fprintf(os.Stderr, "請在遠端執行 tsm daemon start\n")
		os.Exit(1)
	}
	defer c.Close()

	cfg := loadConfig() // 設定不變，移到迴圈外

	// reconnResult 封裝重連結果。
	type reconnResult struct {
		client *client.Client
	}

	for {
		// 顯示 session 選單
		deps := ui.Deps{Client: c, Cfg: cfg, RemoteMode: true}
		m := ui.NewModel(deps)
		p := tea.NewProgram(m, tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fm, ok := finalModel.(ui.Model)
		if !ok {
			return
		}

		var selected string

		if fm.WatchFailed() {
			// Watch stream 斷線 → 直接進入重連流程（無目標 session）
			selected = ""
		} else {
			selected = fm.Selected()
			if selected == "" {
				return // 使用者按 q/esc 退出
			}

			// 連線到遠端 session
			result := remote.Attach(host, selected)
			if result == remote.AttachDetached {
				continue // 正常 detach → 回到選單
			}
		}

		// 斷線 — 顯示重連 modal
		newC := doReconnect(host, selected, tun)
		if newC == nil {
			return // 使用者選擇退出
		}
		c.Close()
		c = newC

		// 重連成功：若有目標 session 則重新 attach，否則回到選單
		if selected != "" {
			result := remote.Attach(host, selected)
			if result == remote.AttachDetached {
				continue
			}
			// 又斷線 → 繼續迴圈重連
		}
	}
}

// doReconnect 顯示重連 modal 並背景嘗試重建 SSH tunnel + gRPC。
// 成功回傳新的 client，使用者放棄回傳 nil。
func doReconnect(host, session string, tun *remote.Tunnel) *client.Client {
	rm := remote.NewReconnectModel(host, session)
	reconnProg := tea.NewProgram(rm, tea.WithAltScreen())

	type reconnResult struct {
		client *client.Client
	}
	resultCh := make(chan reconnResult, 1)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(resultCh)
		for {
			reconnProg.Send(remote.ReconnStateMsg{State: remote.StateConnecting})

			select {
			case <-ctx.Done():
				return
			default:
			}

			tun.Close()
			if err := tun.Start(); err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}

			newC, err := client.DialSocket(tun.LocalSocket())
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}

			dctx, dcancel := context.WithTimeout(ctx, 2*time.Second)
			_, err = newC.DaemonStatus(dctx)
			dcancel()
			if err != nil {
				newC.Close()
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}

			reconnProg.Send(remote.ReconnStateMsg{State: remote.StateConnected})
			resultCh <- reconnResult{client: newC}
			return
		}
	}()

	reconnFinal, err := reconnProg.Run()
	cancel()

	if err != nil {
		return nil
	}

	rfm, ok := reconnFinal.(remote.ReconnectModel)
	if !ok {
		return nil
	}

	// 不論結果，嘗試取得新 client
	var newC *client.Client
	select {
	case res, ok := <-resultCh:
		if ok && res.client != nil {
			newC = res.client
		}
	default:
	}

	if rfm.Quit() {
		if newC != nil {
			newC.Close()
		}
		return nil
	}

	return newC
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
	cfg.InTmux = os.Getenv("TMUX") != ""

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
		handlePostTUI(m)
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
		handlePostTUI(m)
	}
}

// postTUIPath 回傳 post-TUI tmux 指令檔的路徑。
func postTUIPath() string {
	return config.ExpandPath("~/.config/tsm/post-tui.conf")
}

// handlePostTUI 處理 TUI 結束後的動作。
func handlePostTUI(m ui.Model) {
	// 先清除舊的 post-tui 檔案，避免 popup 關閉後執行過期指令
	os.Remove(postTUIPath())

	if selected := m.Selected(); selected != "" {
		switchToSession(selected, m.ReadOnly())
		return
	}
	if m.ExitTmux() && os.Getenv("TMUX") != "" {
		_ = osexec.Command("tmux", "detach-client").Run()
	}
}

func switchToSession(name string, readOnly bool) {
	var err error
	if os.Getenv("TMUX") != "" {
		// 清除殘留的 CLIENT_READONLY（若存在），避免 display-popup 等指令被阻擋
		if out, e := osexec.Command("tmux", "display-message", "-p", "#{client_readonly}").Output(); e == nil {
			if strings.TrimSpace(string(out)) == "1" {
				_ = osexec.Command("tmux", "switch-client", "-r").Run() // toggle off
			}
		}

		if readOnly {
			// 停用 pane 輸入 — 必須在 switch-client 之前執行，
			// 否則從 popup 內執行時會因 popup 關閉而被撤銷
			_ = osexec.Command("tmux", "select-pane", "-t", name, "-d").Run()
		} else {
			// 還原：啟用 pane 輸入（同樣在 switch-client 前）
			_ = osexec.Command("tmux", "select-pane", "-t", name, "-e").Run()
		}

		err = osexec.Command("tmux", "switch-client", "-t", name).Run()
		if err == nil {
			// 修正 iTerm tab 名稱：對目標 session 的 window 重啟 automatic-rename
			_ = osexec.Command("tmux", "set-option", "-w", "-t", name, "automatic-rename", "on").Run()

			// 清除可能殘留的舊版 key-table 設定（升級相容）
			_ = osexec.Command("tmux", "set-option", "-t", name, "-u", "key-table").Run()
		}
	} else {
		args := []string{"attach-session", "-t", name}
		if readOnly {
			args = append(args, "-r")
		}
		cmd := osexec.Command("tmux", args...)
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
	fmt.Fprintln(os.Stderr, "  daemon start       啟動 daemon（背景執行）")
	fmt.Fprintln(os.Stderr, "  daemon stop        停止 daemon")
	fmt.Fprintln(os.Stderr, "  daemon restart     重新啟動 daemon")
	fmt.Fprintln(os.Stderr, "  daemon status      顯示 daemon 狀態")
	fmt.Fprintln(os.Stderr, "  hooks install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  hooks uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "  upgrade            檢查並升級到最新版本")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --version, -v      顯示版本號")
	fmt.Fprintln(os.Stderr, "  --inline           強制使用內嵌全螢幕模式")
	fmt.Fprintln(os.Stderr, "  --remote <host>    透過 SSH 連線遠端主機的 tsm-daemon")
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
		if containsFlag(args[1:], "--foreground") {
			// 前景執行（供 daemonize 的子程序使用）
			d := daemon.NewDaemon(cfg)
			if err := d.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// 背景 daemonize
			if err := daemon.Start(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	case "stop":
		if err := daemon.Stop(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "restart":
		// 若有在跑就先停
		if daemon.IsRunning(cfg) {
			if err := daemon.Stop(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
				os.Exit(1)
			}
		}
		if err := daemon.Start(cfg); err != nil {
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
	fmt.Fprintln(os.Stderr, "  start                啟動 daemon（背景執行）")
	fmt.Fprintln(os.Stderr, "  start --foreground   前景啟動 daemon")
	fmt.Fprintln(os.Stderr, "  stop       停止 daemon")
	fmt.Fprintln(os.Stderr, "  restart    重新啟動 daemon")
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
		return
	}

	m := setup.NewModel(components)
	if isDaemonRunning() {
		m.SetDaemonHint("daemon 需要重新啟動以套用變更")
	} else {
		m.SetDaemonHint("daemon 需要啟動以套用變更")
	}
	m.SetRestartFn(func() error {
		// 抑制 daemon 的 stderr 輸出，避免干擾 Bubble Tea 渲染
		origStderr := os.Stderr
		if devNull, err := os.Open(os.DevNull); err == nil {
			os.Stderr = devNull
			defer func() { os.Stderr = origStderr; devNull.Close() }()
		}
		cfg := loadConfig()
		if daemon.IsRunning(cfg) {
			if err := daemon.Stop(cfg); err != nil {
				return err
			}
		}
		return daemon.Start(cfg)
	})
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runUpgrade() {
	u := upgrade.DefaultUpgrader()

	fmt.Printf("目前版本: %s\n", version.Version)
	fmt.Println("檢查最新版本...")

	rel, err := u.CheckLatest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法取得最新版本: %v\n", err)
		os.Exit(1)
	}

	if !upgrade.NeedsUpgrade(version.Version, rel.Version) {
		fmt.Printf("已是最新版本 v%s\n", version.Version)
		return
	}

	fmt.Printf("發現新版本: v%s\n", rel.Version)

	asset := upgrade.AssetName()
	url, ok := rel.Assets[asset]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: 找不到適用於此平台的檔案 (%s)\n", asset)
		os.Exit(1)
	}

	fmt.Printf("下載 %s ...\n", asset)
	tmpPath, err := u.Download(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 下載失敗: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("啟動安裝程式...")
	if err := u.ExecFunc(tmpPath, "setup"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法執行安裝程式: %v\n", err)
		os.Remove(tmpPath)
		os.Exit(1)
	}
	os.Remove(tmpPath)
}

// isDaemonRunning 檢查 daemon 是否正在運行。
func isDaemonRunning() bool {
	return daemon.IsRunning(loadConfig())
}

func buildSetupComponents() []setup.Component {
	return []setup.Component{
		selfinstall.BuildComponent(),
		bind.BuildComponent(),
		hooks.BuildComponent(),
	}
}
