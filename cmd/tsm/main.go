package main

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/wake/tmux-session-menu/internal/bind"
	"github.com/wake/tmux-session-menu/internal/cfgtui"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/daemon"
	"github.com/wake/tmux-session-menu/internal/hooks"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/launcher"
	"github.com/wake/tmux-session-menu/internal/remote"
	"github.com/wake/tmux-session-menu/internal/selfinstall"
	"github.com/wake/tmux-session-menu/internal/setup"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
	"github.com/wake/tmux-session-menu/internal/upgrade"
	"github.com/wake/tmux-session-menu/internal/upload"
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

// parseHostFlags 從命令列參數中收集所有 --host <value> 的主機名稱。
func parseHostFlags(args []string) []string {
	var hosts []string
	for i, a := range args {
		if a == "--host" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			hosts = append(hosts, args[i+1])
		}
	}
	return hosts
}

// parseLocalFlag 回傳命令列中是否包含 --local 旗標。
func parseLocalFlag(args []string) bool {
	return containsFlag(args, "--local")
}

// hasHostMode 回傳命令列中是否指定了多主機模式（--host 或 --local）。
func hasHostMode(args []string) bool {
	return containsFlag(args, "--host") || containsFlag(args, "--local")
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

	// --host / --local / --inline / --popup 都可能混合出現
	if hasHostMode(args) || args[0] == "--inline" || args[0] == "--popup" {
		runWithMode(parseRunMode(args))
		return
	}

	// 子命令保持不變
	if args[0] == "bind" {
		runBind(args[1:])
		return
	}
	if args[0] == "hooks" {
		runHooks(args[1:])
		return
	}
	if args[0] == "status-name" {
		runStatusName()
		return
	}
	if args[0] == "daemon" {
		runDaemon(args[1:])
		return
	}
	if args[0] == "config" {
		runConfig()
		return
	}
	if args[0] == "iterm-coprocess" {
		runItermCoprocess(args[1:])
		return
	}
	if args[0] == "setup" {
		runSetup(args[1:])
		return
	}
	if args[0] == "upgrade" {
		force := containsFlag(args[1:], "--force") || containsFlag(args[1:], "-f")
		silent := containsFlag(args[1:], "--silent")
		if silent {
			runSilentUpgrade()
			return
		}
		runUpgrade(force)
		return
	}

	printUsage()
	os.Exit(1)
}

// runWithMode 根據指定模式啟動 TUI
func runWithMode(mode runMode) {
	// Client mode：顯示 launcher 選單
	dataDir := config.ExpandPath(config.Default().DataDir)
	if config.LoadInstallMode(dataDir) == config.ModeClient {
		runClientLauncher(dataDir)
		return
	}

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
		// 轉送 CLI 旗標到 popup 子程序（排除 --popup 本身）
		popupArgs := []string{exe, "--inline"}
		for _, a := range os.Args[1:] {
			if a != "--popup" {
				popupArgs = append(popupArgs, a)
			}
		}
		cmd := osexec.Command("tmux", append([]string{"display-popup", "-E", "-w", "80%", "-h", "80%"}, popupArgs...)...)
		cmd.Env = append(os.Environ(), "TSM_IN_POPUP=1")
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

func runClientLauncher(dataDir string) {
	placeholder := config.LoadLastRemoteHost(dataDir)
	m := launcher.NewModel(placeholder)
	p := tea.NewProgram(m)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fm, ok := finalModel.(launcher.Model)
	if !ok || fm.Choice() == launcher.ChoiceNone {
		return
	}

	switch fm.Choice() {
	case launcher.ChoiceRemote:
		host := fm.Host()
		_ = config.SaveLastRemoteHost(dataDir, host)
		runRemote(host)

	case launcher.ChoiceLocal:
		runTUI()

	case launcher.ChoiceFullSetup:
		// 切換到 full mode 並執行 setup
		_ = config.SaveInstallMode(dataDir, config.ModeFull)
		runSetup(nil)
		runTUI()
	}
}

// runTUI 是所有 TUI 模式的統一入口。
// 無論是無參數啟動（local only）、--host/--local 多主機模式，或 client launcher 呼叫，
// 一律建立 HostManager 管理所有主機的連線與快照聚合。
func runTUI() {
	// 防禦性清理：移除可能殘留的舊 exit-requested sentinel 檔案
	remote.RemoveExitMarker()

	cfg := loadConfig()
	cfg.InTmux = os.Getenv("TMUX") != ""
	cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"

	ensureLocalStatusBar(cfg)

	args := os.Args[1:]
	hostFlags := parseHostFlags(args)
	localFlag := parseLocalFlag(args)
	hostMode := hasHostMode(args)

	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")

	// 決定 host 清單
	if hostMode && (len(hostFlags) > 0 || localFlag) {
		// 有明確旗標 → merge + 存 config
		merged := config.MergeHosts(cfg.Hosts, hostFlags, localFlag)
		cfg.Hosts = merged
		_ = config.SaveConfig(cfgPath, cfg)
	} else if !hostMode {
		// tsm 無參數 → 強制只啟用 local，不存 config
		for i := range cfg.Hosts {
			if cfg.Hosts[i].IsLocal() {
				cfg.Hosts[i].Enabled = true
			} else {
				cfg.Hosts[i].Enabled = false
			}
		}
	}
	// else: tsm --host 裸用 → 直接使用 config 中的 enabled 狀態（不存 config）

	// 防禦性確保 local 永遠存在
	cfg.Hosts = config.EnsureLocal(cfg.Hosts)

	// 建立 HostManager
	mgr := hostmgr.New()
	for _, h := range cfg.Hosts {
		mgr.AddHost(h)
	}
	for _, h := range mgr.Hosts() {
		if h.IsLocal() {
			h.SetGlobalConfig(cfg)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	defer mgr.Close()
	exec := tmux.NewRealExecutor()
	upgrader := upgrade.DefaultUpgrader()

	for {
		deps := ui.Deps{
			HostMgr:    mgr,
			Cfg:        cfg,
			ConfigPath: cfgPath,
			Upgrader:   upgrader,
		}
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

		if fm.UpgradeReady() {
			cancel()
			runPostUpgrade(fm, cfg)
			return
		}
		if fm.OpenConfig() {
			runConfig()
			cfg = loadConfig()
			cfg.InTmux = os.Getenv("TMUX") != ""
			cfg.InPopup = os.Getenv("TSM_IN_POPUP") == "1"
			continue
		}

		selected := fm.Selected()
		if selected == "" {
			if fm.ExitTmux() && os.Getenv("TMUX") != "" {
				_ = remote.WriteExitMarker()
				_ = osexec.Command("tmux", "detach-client").Run()
			}
			return
		}

		// 判斷選取的 session 屬於哪台主機
		item := fm.SelectedItem()
		host := mgr.Host(item.HostID)

		if host == nil || host.IsLocal() {
			// 本機 session
			switchToSession(selected, fm.ReadOnly())
			if cfg.InPopup {
				return
			}
			continue
		}

		// 遠端 session — attach
		hostCfg := host.Config()
		applyHostBar := func() {
			if hostCfg.Color != "" {
				barCfg := config.ColorConfig{
					BarBG:   hostCfg.Color,
					BadgeBG: hostCfg.Color,
					BadgeFG: cfg.Remote.BadgeFG,
				}
				_ = tmux.ApplyStatusBar(exec, barCfg)
			} else {
				_ = tmux.ApplyStatusBar(exec, cfg.Remote)
			}
		}

		applyHostBar()
		result := remote.Attach(hostCfg.Address, selected)
		_ = tmux.ApplyStatusBar(exec, cfg.Local)

		if result == remote.AttachDetached {
			if remote.CheckAndClearExitRequested(hostCfg.Address) {
				return
			}
			continue
		}

		// 斷線重連
		for {
			if !doMultiHostReconnect(mgr, item.HostID, selected) {
				break
			}
			applyHostBar()
			result = remote.Attach(hostCfg.Address, selected)
			_ = tmux.ApplyStatusBar(exec, cfg.Local)
			if result == remote.AttachDetached {
				if remote.CheckAndClearExitRequested(hostCfg.Address) {
					return
				}
				break
			}
		}
	}
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

	cfg := loadConfig()            // 設定不變，移到迴圈外
	exec := tmux.NewRealExecutor() // status bar 切換用

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

			// 套用 remote status bar 樣式
			_ = tmux.ApplyStatusBar(exec, cfg.Remote)

			// 連線到遠端 session
			result := remote.Attach(host, selected)

			// 恢復 local status bar 樣式
			_ = tmux.ApplyStatusBar(exec, cfg.Local)

			if result == remote.AttachDetached {
				// 檢查遠端是否有 ctrl+e 的退出信號
				if remote.CheckAndClearExitRequested(host) {
					return // 使用者按 ctrl+e → 退出整個遠端模式
				}
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
		for selected != "" {
			// 套用 remote status bar 樣式
			_ = tmux.ApplyStatusBar(exec, cfg.Remote)

			result := remote.Attach(host, selected)

			// 恢復 local status bar 樣式
			_ = tmux.ApplyStatusBar(exec, cfg.Local)

			if result == remote.AttachDetached {
				if remote.CheckAndClearExitRequested(host) {
					return // 使用者按 ctrl+e → 退出整個遠端模式
				}
				break // 正常 detach → 回到選單
			}

			// 又斷線 → 再次重連
			newC2 := doReconnect(host, selected, tun)
			if newC2 == nil {
				return // 使用者選擇退出
			}
			c.Close()
			c = newC2
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

			// 先寫入 resultCh，再通知 TUI 結束，避免 race condition：
			// 若先 Send 再寫 channel，TUI 可能在 goroutine 寫入前就退出，
			// 導致 non-blocking select 讀不到結果。
			resultCh <- reconnResult{client: newC}
			reconnProg.Send(remote.ReconnStateMsg{State: remote.StateConnected})
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

	// 嘗試取得新 client（短暫等待以防 goroutine 尚未寫入）
	var newC *client.Client
	select {
	case res, ok := <-resultCh:
		if ok && res.client != nil {
			newC = res.client
		}
	case <-time.After(2 * time.Second):
	}

	if rfm.Quit() {
		if newC != nil {
			newC.Close()
		}
		return nil
	}

	return newC
}

// doMultiHostReconnect 顯示重連 popup 並等待 HostManager 自動重連指定主機。
// 成功回傳 true，使用者放棄回傳 false。
func doMultiHostReconnect(mgr *hostmgr.HostManager, hostID, session string) bool {
	rm := remote.NewReconnectModel(hostID, session)
	reconnProg := tea.NewProgram(rm, tea.WithAltScreen())

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		attempt := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}

			h := mgr.Host(hostID)
			if h == nil {
				continue
			}

			attempt++
			if h.Status() == hostmgr.HostConnected {
				reconnProg.Send(remote.ReconnStateMsg{
					State:   remote.StateConnected,
					Attempt: attempt,
				})
				return
			}

			reconnProg.Send(remote.ReconnStateMsg{
				State:   remote.StateConnecting,
				Attempt: attempt,
			})
		}
	}()

	reconnFinal, err := reconnProg.Run()
	cancel()

	if err != nil {
		return false
	}

	rfm, ok := reconnFinal.(remote.ReconnectModel)
	if !ok {
		return false
	}

	if rfm.Quit() || rfm.BackToMenu() {
		return false
	}

	return true
}

// runStatusName 輸出當前 session 的自訂名稱，供 tmux status bar 使用。
// 錯誤一律靜默處理，因為 tmux 呼叫時不應顯示錯誤。
func runStatusName() {
	out, err := osexec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return
	}
	sessionName := strings.TrimSpace(string(out))
	if sessionName == "" {
		return
	}

	cfg := loadConfig()
	dataDir := config.ExpandPath(cfg.DataDir)
	dbPath := filepath.Join(dataDir, "state.db")

	st, err := store.Open(dbPath)
	if err != nil {
		return
	}
	defer st.Close()

	name, err := st.GetCustomName(sessionName)
	if err != nil || name == "" {
		return
	}

	fmt.Print(name)
}

// runConfig 啟動設定 TUI，儲存後寫入 config.toml。
func runConfig() {
	cfg := loadConfig()
	m := cfgtui.NewModel(cfg)

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fm, ok := finalModel.(cfgtui.Model)
	if !ok || !fm.Saved() {
		return
	}

	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	result := fm.ResultConfig()
	result.DataDir = cfg.DataDir
	if err := config.SaveConfig(cfgPath, result); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 儲存設定失敗: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("設定已儲存")
}

// ensureLocalStatusBar 確保 status-left 格式為最新（使用 per-session #{@tsm_name}）。
func ensureLocalStatusBar(cfg config.Config) {
	if cfg.InTmux {
		exec := tmux.NewRealExecutor()
		_ = tmux.ApplyStatusBar(exec, cfg.Local)
	}
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

			// 套用 local status bar 樣式
			cfg := loadConfig()
			exec := tmux.NewRealExecutor()
			_ = tmux.ApplyStatusBar(exec, cfg.Local)
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
	fmt.Fprintln(os.Stderr, "  config             設定 tsm 參數與主題色")
	fmt.Fprintln(os.Stderr, "  setup              互動式安裝所有元件")
	fmt.Fprintln(os.Stderr, "  bind install       安裝 Ctrl+Q 快捷鍵到 ~/.tmux.conf")
	fmt.Fprintln(os.Stderr, "  bind uninstall     移除 Ctrl+Q 快捷鍵")
	fmt.Fprintln(os.Stderr, "  daemon start       啟動 daemon（背景執行）")
	fmt.Fprintln(os.Stderr, "  daemon stop        停止 daemon")
	fmt.Fprintln(os.Stderr, "  daemon restart     重新啟動 daemon")
	fmt.Fprintln(os.Stderr, "  daemon status      顯示 daemon 狀態")
	fmt.Fprintln(os.Stderr, "  hooks install      安裝 tsm hooks 到 Claude Code settings")
	fmt.Fprintln(os.Stderr, "  hooks uninstall    移除 tsm hooks")
	fmt.Fprintln(os.Stderr, "  status-name        輸出當前 session 自訂名稱（供 tmux status bar 使用）")
	fmt.Fprintln(os.Stderr, "  upgrade [--force] [--silent]  檢查並升級到最新版本")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --version, -v      顯示版本號")
	fmt.Fprintln(os.Stderr, "  --inline           強制使用內嵌全螢幕模式")
	fmt.Fprintln(os.Stderr, "  --host             啟動多主機模式（讀取設定中已啟用的主機）")
	fmt.Fprintln(os.Stderr, "  --host <name>      啟動指定主機（可多次使用，覆寫設定）")
	fmt.Fprintln(os.Stderr, "  --local            僅啟動本地端（可搭配 --host 使用，覆寫設定）")
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

	dataDir := config.ExpandPath(config.Default().DataDir)
	_ = os.MkdirAll(dataDir, 0o755)

	// 載入上次的安裝模式
	savedMode := config.LoadInstallMode(dataDir)

	m := setup.NewModel(components)

	// 將 config.InstallMode 映射到 setup.InstallMode
	if savedMode == config.ModeClient {
		m.SetInstallMode(setup.ModeClient)
	}

	// 只在 full 模式設定 daemon 提示
	if savedMode == config.ModeFull {
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
	}
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// 儲存使用者選擇的安裝模式
	if fm, ok := finalModel.(setup.Model); ok {
		cfgMode := config.ModeFull
		if fm.InstallMode() == setup.ModeClient {
			cfgMode = config.ModeClient
		}
		_ = config.SaveInstallMode(dataDir, cfgMode)
	}
}

func runUpgrade(force bool) {
	u := upgrade.DefaultUpgrader()

	fmt.Printf("目前版本: %s\n", version.Version)
	fmt.Println("檢查最新版本...")

	rel, err := u.CheckLatest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: 無法取得最新版本: %v\n", err)
		os.Exit(1)
	}

	if !force && !upgrade.NeedsUpgrade(version.Version, rel.Version) {
		fmt.Printf("已是最新版本 v%s\n", version.Version)
		return
	}

	if force && !upgrade.NeedsUpgrade(version.Version, rel.Version) {
		fmt.Printf("版本相同 v%s，強制升級\n", rel.Version)
	} else {
		fmt.Printf("發現新版本: v%s\n", rel.Version)
	}

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

// runSilentUpgrade 執行非互動升級（供遠端 SSH 呼叫）。
// stdout 只印一行版本號，錯誤走 stderr。不做 syscall.Exec。
// atomicInstallBinary 以 atomic rename 安裝下載的 binary 到偵測到的安裝路徑。
func atomicInstallBinary(tmpPath string) (string, error) {
	det := selfinstall.Detect()
	targetDir := filepath.Dir(det.Path)
	if targetDir == "" || targetDir == "." {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("無法取得 home 目錄: %w", err)
		}
		targetDir = filepath.Join(home, ".local", "bin")
	}
	target := filepath.Join(targetDir, "tsm")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("無法建立目錄: %w", err)
	}
	// Atomic: 複製 tmp → target.tmp，再 rename → target
	tmpTarget := target + ".tmp"
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("讀取下載檔案失敗: %w", err)
	}
	if err := os.WriteFile(tmpTarget, data, 0o755); err != nil {
		return "", fmt.Errorf("寫入暫存檔失敗: %w", err)
	}
	if err := os.Rename(tmpTarget, target); err != nil {
		os.Remove(tmpTarget)
		return "", fmt.Errorf("替換執行檔失敗: %w", err)
	}
	return target, nil
}

func runSilentUpgrade() {
	u := upgrade.DefaultUpgrader()
	cfg := loadConfig()

	ops := upgrade.SilentOps{
		CurrentVersion: upgrade.ExtractSemver(version.Version),
		CheckLatest:    u.CheckLatest,
		Download:       u.Download,
		InstallBinary:  atomicInstallBinary,
		RemoveTmp: func(path string) {
			os.Remove(path)
		},
		StopDaemon: func() error {
			if daemon.IsRunning(cfg) {
				return daemon.Stop(cfg)
			}
			return nil
		},
		StartDaemon: func() error {
			return daemon.Start(cfg)
		},
	}

	result, err := upgrade.RunSilent(ops)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println(result.Version)
}

// runPostUpgrade 執行 TUI 退出後的升級流程。
// 支援兩種模式：
//   - 一般升級（tmpPath != ""）：安裝下載的 binary → 重啟 daemon → exec
//   - 僅重啟（binPath != ""）：磁碟上已是新版，跳過安裝 → 重啟 daemon → exec
func runPostUpgrade(m ui.Model, cfg config.Config) {
	tmpPath, ver := m.UpgradeInfo()
	binPath := m.UpgradeBinPath()

	// restart-only 模式：binary 已由其他 tab 更新
	if tmpPath == "" && binPath != "" {
		fmt.Fprintf(os.Stderr, "重新啟動至 v%s...\n", ver)
		runRestartOnly(binPath, cfg)
		return
	}

	if tmpPath == "" {
		fmt.Fprintln(os.Stderr, "升級失敗: 無下載檔案且無已安裝路徑")
		os.Exit(1)
	}

	ops := upgrade.PostUpgradeOps{
		InstallBinary: atomicInstallBinary,
		RemoveTmp: func(path string) {
			os.Remove(path)
		},
		StopDaemon: func() error {
			if daemon.IsRunning(cfg) {
				return daemon.Stop(cfg)
			}
			return nil
		},
		StartDaemon: func() error {
			return daemon.Start(cfg)
		},
		Exec: func(path string, args []string, env []string) error {
			return syscall.Exec(path, args, env)
		},
	}

	fmt.Fprintf(os.Stderr, "升級至 v%s，重新啟動...\n", ver)
	if err := upgrade.RunPostUpgrade(ops, tmpPath, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "升級失敗: %v\n", err)
		os.Exit(1)
	}
}

// runRestartOnly 僅重啟 daemon 並 exec 到已安裝的新版 binary（無需下載安裝）。
func runRestartOnly(binPath string, cfg config.Config) {
	// daemon stop 失敗不阻斷流程（可能本來就沒跑）
	if daemon.IsRunning(cfg) {
		_ = daemon.Stop(cfg)
	}

	if err := daemon.Start(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "啟動 daemon 失敗: %v\n", err)
		os.Exit(1)
	}

	if err := syscall.Exec(binPath, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec 失敗（binary: %s）: %v\n", binPath, err)
		os.Exit(1)
	}
}

// isDaemonRunning 檢查 daemon 是否正在運行。
func isDaemonRunning() bool {
	return daemon.IsRunning(loadConfig())
}

func buildSetupComponents() []setup.Component {
	bindComp := bind.BuildComponent()
	bindComp.FullOnly = true
	hooksComp := hooks.BuildComponent()
	hooksComp.FullOnly = true
	components := []setup.Component{
		selfinstall.BuildComponent(),
		bindComp,
		hooksComp,
	}

	if setup.DetectIterm2() {
		components = append(components, setup.Component{
			Label:       "iTerm2 拖曳上傳",
			InstallFn:   setup.InstallItermCoprocess,
			UninstallFn: setup.UninstallItermCoprocess,
			Checked:     true,
			Installed:   setup.DetectItermCoprocessInstalled(),
			Note:        "設定 fileDropCoprocess，支援拖曳檔案自動上傳到遠端",
		})
	}

	return components
}

// runItermCoprocess 執行 iTerm2 fileDropCoprocess 邏輯。
func runItermCoprocess(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tsm iterm-coprocess <filenames>")
		os.Exit(1)
	}
	filenames := upload.ReconstructFilenames(args)
	if len(filenames) == 0 {
		return
	}
	upload.RunCoprocess(filenames)
}
