package ui

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/ai"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/selfinstall"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/upgrade"
	"github.com/wake/tmux-session-menu/internal/version"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Deps 封裝 Model 的外部依賴。
// 當 HostMgr 不為 nil 時使用多主機模式；
// 當 Client 不為 nil 時使用 gRPC daemon 模式；
// 否則使用舊的 TmuxMgr+Store 直接模式（主要用於測試）。
type Deps struct {
	// 多主機模式
	HostMgr    *hostmgr.HostManager
	ConfigPath string // config.toml 路徑，供 persistHosts 使用

	// 舊模式（保留向下相容）
	Client     *client.Client // gRPC client（daemon 模式）
	TmuxMgr    *tmux.Manager  // 舊模式：直接 tmux 操作
	Store      *store.Store   // 舊模式：直接 SQLite 操作
	Cfg        config.Config
	StatusDir  string
	RemoteMode bool // 遠端模式：Watch 錯誤時退出而非重試

	Upgrader *upgrade.Upgrader // nil 時 ctrl+u 無效
	HubMode  bool              // true = 使用 WatchMultiHost（hub 模式）
}

// persistHosts 將 HostManager 的主機清單寫回 config.toml。
func (m Model) persistHosts() {
	if m.deps.HostMgr == nil || m.deps.ConfigPath == "" {
		return
	}
	hosts := m.deps.HostMgr.Hosts()
	entries := make([]config.HostEntry, len(hosts))
	for i, h := range hosts {
		cfg := h.Config()
		cfg.SortOrder = i
		entries[i] = cfg
	}

	fileCfg := config.Default()
	cfgPath := m.deps.ConfigPath
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			fileCfg = loaded
		}
	}
	fileCfg.Hosts = entries
	_ = config.SaveConfig(cfgPath, fileCfg)
}

// AnimTickMsg 表示動畫定時更新觸發。
type AnimTickMsg struct{}

// Model 是 Bubble Tea 的主要模型。
type Model struct {
	width    int
	height   int
	cursor   int
	items    []ListItem
	quitting bool

	deps           Deps
	selected       string // Enter 選取的 session name
	selectedHostID string // Enter 選取的 session 所屬主機 ID（多主機模式）
	readOnly_      bool   // R 鍵：唯讀進入 session
	exitTmux_      bool   // ctrl+e：退出 tmux
	openConfig_    bool   // c 鍵：開啟 config TUI
	watchFailed    bool // remote 模式 Watch stream 失敗
	hubDegraded    bool // hub 模式 daemon 不再支援 → 降級到 HostManager
	err            error

	// 動畫
	animFrame int // running 狀態閃動用的 frame 計數器

	// Mode 相關
	mode        Mode
	inputTarget InputTarget
	inputPrompt string
	textInput   textinput.Model

	// 雙行輸入（用於 Session rename）
	textInputs [2]textinput.Model // [0]=名稱(CustomName), [1]=ID(session name)
	inputRow   int                // 目前焦點行 0 or 1

	confirmPrompt     string
	confirmAction     func() tea.Cmd
	confirmExitTmux   bool // 確認後是否退出 tmux
	confirmReturnMode Mode // 確認/取消後回到哪個模式（預設 ModeNormal）

	// Picker mode（選擇群組）
	pickerGroups []store.Group
	pickerCursor int
	pickerTarget string // 被移動的 session name

	// Search mode（搜尋）
	searchQuery string

	// HostPicker mode（主機管理面板）
	hostPickerCursor int
	hubHostSnap      *tsmv1.MultiHostSnapshot // hub 模式：最新的主機快照（供 host picker 使用）

	// host picker 右側面板
	hostPanelOpen    bool
	hostPanelCursor  int                      // 0=[x]啟用, 1=bar_bg, 2=bar_fg, 3=badge_bg, 4=badge_fg
	hostPanelEditing bool                     // 正在編輯某個色彩欄位
	hostPanelDraft   map[string]hostDraftEntry // hostID → draft copy of color edits
	hostSavedMsg     string                   // "已儲存" flash message

	// NewSession 表單
	newSession newSessionForm

	// 升級狀態
	upgradeReady    bool
	upgradeTmpPath  string
	upgradeBinPath  string // restart-only 時直接 exec 的路徑
	upgradeVersion  string
	upgradeChecking bool // 防止 ctrl+u 重複觸發

	// 升級面板
	upgradeItems     []upgradeItem
	upgradeCursor    int
	upgradeRunning   bool
	upgradeCancelled bool
	upgradeLatestVer string
	upgradeBtnFocus  int // 0=升級按鈕, 1=取消按鈕
	upgradeGen       int // 升級輪次，用於丟棄過期的 RemoteUpgradeMsg

	// 上傳模式
	uploadModal uploadModal
}

// NewModel 建立初始 Model。
func NewModel(deps Deps) Model {
	ti := textinput.New()
	ti.PromptStyle = selectedStyle
	ti.TextStyle = lipgloss.NewStyle()
	ti.Cursor.Style = selectedStyle

	var tis [2]textinput.Model
	for i := range tis {
		tis[i] = textinput.New()
		tis[i].PromptStyle = selectedStyle
		tis[i].TextStyle = lipgloss.NewStyle()
		tis[i].Cursor.Style = selectedStyle
	}

	return Model{deps: deps, textInput: ti, textInputs: tis}
}

// SessionsMsg 是 loadSessions 完成後的回傳訊息。
type SessionsMsg struct {
	Sessions []tmux.Session
	Groups   []store.Group
	Err      error
}

// CheckUpgradeMsg 版本檢查結果。
type CheckUpgradeMsg struct {
	Release       *upgrade.Release
	Err           error
	InstalledVer  string            // 磁碟上已安裝的版本（semver 部分）
	InstalledPath string            // 已安裝的路徑
	HostVersions  map[string]string // hostID → version（遠端 DaemonStatus 取得）
}

// DownloadUpgradeMsg 下載結果。
type DownloadUpgradeMsg struct {
	TmpPath string
	Err     error
}

// checkUpgradeCmd 回傳一個 Bubble Tea Cmd，非同步呼叫 Upgrader.CheckLatest 並偵測已安裝版本。
// 當 mgr 不為 nil 時，會並行收集各主機的版本資訊。
func checkUpgradeCmd(u *upgrade.Upgrader, mgr *hostmgr.HostManager) tea.Cmd {
	return func() tea.Msg {
		rel, err := u.CheckLatest()

		// 同時偵測磁碟上已安裝的版本
		det := selfinstall.Detect()
		var installedVer, installedPath string
		if det.Version != "" {
			installedVer = upgrade.ExtractSemver(det.Version)
			installedPath = det.Path
		}

		// 收集各主機版本
		var hostVersions map[string]string
		if mgr != nil {
			hostVersions = make(map[string]string)
			hosts := mgr.Hosts()
			var mu sync.Mutex
			var wg sync.WaitGroup
			for _, h := range hosts {
				if h.IsLocal() {
					hostVersions[h.ID()] = upgrade.ExtractSemver(version.Version)
					continue
				}
				c := h.Client()
				if c == nil {
					hostVersions[h.ID()] = ""
					continue
				}
				wg.Add(1)
				go func(hostID string, cl *client.Client) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					resp, rpcErr := cl.DaemonStatus(ctx)
					mu.Lock()
					defer mu.Unlock()
					if rpcErr != nil || resp.Version == "" {
						hostVersions[hostID] = ""
					} else {
						hostVersions[hostID] = upgrade.ExtractSemver(resp.Version)
					}
				}(h.ID(), c)
			}
			wg.Wait()
		}

		if err != nil {
			return CheckUpgradeMsg{Err: err, InstalledVer: installedVer, InstalledPath: installedPath, HostVersions: hostVersions}
		}
		return CheckUpgradeMsg{Release: &rel, InstalledVer: installedVer, InstalledPath: installedPath, HostVersions: hostVersions}
	}
}

// downloadUpgradeCmd 回傳一個 Bubble Tea Cmd，非同步下載升級 binary。
func downloadUpgradeCmd(u *upgrade.Upgrader, url string) tea.Cmd {
	return func() tea.Msg {
		path, err := u.Download(url)
		if err != nil {
			return DownloadUpgradeMsg{Err: err}
		}
		return DownloadUpgradeMsg{TmpPath: path}
	}
}

// offerRestart 在磁碟上已安裝較新版本時，提供無需下載的重啟確認。
func (m Model) offerRestart(installedVer, installedPath string) Model {
	m.upgradeVersion = installedVer
	m.upgradeBinPath = installedPath
	m.mode = ModeConfirm
	m.confirmPrompt = fmt.Sprintf("已安裝 v%s（目前 TUI 為 v%s），重新啟動？", installedVer, version.Version)
	m.confirmAction = func() tea.Cmd {
		return func() tea.Msg {
			return DownloadUpgradeMsg{} // 空 TmpPath 表示 restart-only
		}
	}
	return m
}

// Items 回傳目前的列表項目。
func (m Model) Items() []ListItem {
	return m.items
}

// Selected 回傳使用者按 Enter 選取的 session name（空字串表示未選取）。
func (m Model) Selected() string {
	return m.selected
}

// WatchFailed 回傳 remote 模式下 Watch stream 是否失敗。
func (m Model) WatchFailed() bool { return m.watchFailed }

// ExitTmux 回傳使用者是否按了 ctrl+e 要求退出 tmux。
func (m Model) ExitTmux() bool {
	return m.exitTmux_
}

// ReadOnly 回傳使用者是否按了 R 要求唯讀進入 session。
func (m Model) ReadOnly() bool {
	return m.readOnly_
}

// OpenConfig 回傳使用者是否按了 c 要求開啟 config TUI。
func (m Model) OpenConfig() bool {
	return m.openConfig_
}

// Quitting 回傳 TUI 是否正在結束。
func (m Model) Quitting() bool {
	return m.quitting
}

// AnimFrame 回傳目前的動畫 frame（主要用於測試）。
func (m Model) AnimFrame() int {
	return m.animFrame
}

// Mode 回傳目前的互動模式。
func (m Model) Mode() Mode {
	return m.mode
}

// InputTarget 回傳目前的輸入目標（主要用於測試）。
func (m Model) InputTarget() InputTarget {
	return m.inputTarget
}

// InputValue 回傳目前的輸入值（主要用於測試）。
func (m Model) InputValue() string {
	return m.textInput.Value()
}

// InputRow 回傳目前雙行輸入的焦點行（主要用於測試）。
func (m Model) InputRow() int {
	return m.inputRow
}

// HostPickerCursor 回傳主機管理面板的游標位置（主要用於測試）。
func (m Model) HostPickerCursor() int {
	return m.hostPickerCursor
}

// DualInputValue 回傳雙行輸入指定行的值（主要用於測試）。
func (m Model) DualInputValue(row int) string {
	if row < 0 || row > 1 {
		return ""
	}
	return m.textInputs[row].Value()
}

// Err 回傳目前的錯誤（主要用於測試）。
func (m Model) Err() error {
	return m.err
}

// NewSessionNameValue 回傳 ModeNewSession 表單的名稱欄位值（主要用於測試）。
func (m Model) NewSessionNameValue() string {
	return m.newSession.nameInput.Value()
}

// SearchQuery 回傳目前的搜尋字串（主要用於測試）。
func (m Model) SearchQuery() string {
	return m.searchQuery
}

// HubDegraded 回傳 hub 模式是否因 daemon 不支援而需要降級到 HostManager。
func (m Model) HubDegraded() bool { return m.hubDegraded }

// connectionMode 回傳目前的連線模式名稱，用於標題列顯示。
func (m Model) connectionMode() string {
	switch {
	case m.deps.HubMode && m.deps.Client != nil && !m.hubDegraded:
		return "Hub"
	case m.deps.HostMgr != nil:
		return "HostManager"
	case m.deps.Client != nil:
		return "Daemon"
	default:
		return "Direct"
	}
}

// UpgradeReady 回傳是否有已下載完成可安裝的升級。
func (m Model) UpgradeReady() bool { return m.upgradeReady }

// UpgradeInfo 回傳升級暫存檔路徑與目標版本。
func (m Model) UpgradeInfo() (tmpPath, version string) { return m.upgradeTmpPath, m.upgradeVersion }

// UpgradeBinPath 回傳 restart-only 時已安裝 binary 的路徑（無需下載）。
func (m Model) UpgradeBinPath() string { return m.upgradeBinPath }

// ConfirmPrompt 回傳目前確認對話框的提示文字。
func (m Model) ConfirmPrompt() string { return m.confirmPrompt }

// visibleItems 回傳目前可見的項目（搜尋時過濾）。
func (m Model) visibleItems() []ListItem {
	if m.mode != ModeSearch || m.searchQuery == "" {
		return m.items
	}
	query := strings.ToLower(m.searchQuery)
	var filtered []ListItem
	for _, item := range m.items {
		if item.Type == ItemGroup {
			continue
		}
		if strings.Contains(strings.ToLower(item.Session.DisplayName()), query) ||
			strings.Contains(strings.ToLower(item.Session.Name), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// loadSessionsCmd 建立一個 tea.Cmd，透過 TmuxMgr 載入 session 列表。
func loadSessionsCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		if deps.TmuxMgr == nil {
			return SessionsMsg{}
		}
		sessions, err := deps.TmuxMgr.ListSessions()
		if err != nil {
			return SessionsMsg{Err: err}
		}

		// 擷取 pane 內容的行數，預設 150
		previewLines := deps.Cfg.PreviewLines
		if previewLines <= 0 {
			previewLines = 150
		}

		// 第二層：一次取得所有 pane title
		paneTitles := make(map[string]string)
		if deps.TmuxMgr != nil {
			if titles, err := deps.TmuxMgr.ListPaneTitles(); err == nil {
				paneTitles = titles
			}
		}

		// 狀態偵測與 AI 模型偵測
		for i := range sessions {
			var paneContent string
			if deps.TmuxMgr != nil {
				if content, err := deps.TmuxMgr.CapturePane(sessions[i].Name, previewLines); err == nil {
					paneContent = content
				}
			}
			paneTitle := paneTitles[sessions[i].Name]
			result := detectSessionStatus(deps, sessions[i].Name, paneTitle, paneContent)
			sessions[i].Status = result.Status
			sessions[i].AiType = result.AiType
			sessions[i].AIModel = ai.DetectModel(paneContent)
		}

		// 從 store 載入 groups 和 session metas
		var groups []store.Group
		if deps.Store != nil {
			groups, _ = deps.Store.ListGroups()
			metas, _ := deps.Store.ListAllSessionMetas()

			groupMap := make(map[int64]string)
			for _, g := range groups {
				groupMap[g.ID] = g.Name
			}

			metaMap := make(map[string]int64)
			customNameMap := make(map[string]string)
			sortOrderMap := make(map[string]int)
			for _, meta := range metas {
				metaMap[meta.SessionName] = meta.GroupID
				sortOrderMap[meta.SessionName] = meta.SortOrder
				if meta.CustomName != "" {
					customNameMap[meta.SessionName] = meta.CustomName
				}
			}

			for i := range sessions {
				if gid, ok := metaMap[sessions[i].Name]; ok {
					if name, ok := groupMap[gid]; ok {
						sessions[i].GroupName = name
					}
				}
				if so, ok := sortOrderMap[sessions[i].Name]; ok {
					sessions[i].SortOrder = so
				}
				if cn, ok := customNameMap[sessions[i].Name]; ok {
					sessions[i].CustomName = cn
				}
			}
		}

		return SessionsMsg{Sessions: sessions, Groups: groups}
	}
}

// SnapshotMsg 是從 gRPC Watch stream 接收的快照訊息。
type SnapshotMsg struct {
	Snapshot *tsmv1.StateSnapshot
	Err      error
}

// reconnectMsg 觸發 Watch stream 重連。
type reconnectMsg struct{}

// hubReconnectMsg 觸發 WatchMultiHost stream 重連。
type hubReconnectMsg struct{}

// errMsg 封裝非同步操作回傳的錯誤。
type errMsg struct{ err error }

// TickMsg 表示定時更新觸發（舊模式使用）。
type TickMsg struct{}

// tickCmd 排程下一次 tick。
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// animTickCmd 排程下一次動畫 tick（50ms 週期用於平滑呼吸效果）。
func animTickCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// watchCmd 啟動 Watch stream 並持續接收快照。
func watchCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		if err := c.Watch(context.Background()); err != nil {
			return SnapshotMsg{Err: err}
		}
		return recvSnapshotCmd(c)()
	}
}

// recvSnapshotCmd 從 Watch stream 接收下一個快照。
func recvSnapshotCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		snap, err := c.RecvSnapshot()
		if err != nil {
			return SnapshotMsg{Err: err}
		}
		return SnapshotMsg{Snapshot: snap}
	}
}

// MultiHostSnapshotMsg 是多主機模式下從 HostManager 接收的聚合快照訊息。
type MultiHostSnapshotMsg struct {
	Snapshots    []HostSnapshotInput    // 使用 items.go 定義的 UI 端型別
	UploadEvents []*tsmv1.UploadEvent   // 聚合所有主機的上傳事件
}

// recvMultiHostCmd 等待 HostManager 快照通知，回傳 MultiHostSnapshotMsg。
// 當 SnapshotCh 被關閉（HostManager.Close）時回傳 nil 避免無窮迴圈。
func recvMultiHostCmd(mgr *hostmgr.HostManager) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-mgr.SnapshotCh()
		if !ok {
			return nil
		}
		return buildMultiHostMsg(mgr)
	}
}

// buildMultiHostMsg 將 hostmgr.HostSnapshot 轉換為 MultiHostSnapshotMsg。
func buildMultiHostMsg(mgr *hostmgr.HostManager) MultiHostSnapshotMsg {
	hostSnaps := mgr.Snapshot()
	var inputs []HostSnapshotInput
	var uploadEvents []*tsmv1.UploadEvent
	for _, hs := range hostSnaps {
		input := HostSnapshotInput{
			HostID: hs.HostID,
			Name:   hs.Name,
			Color:  hs.Color,
			Status: int(hs.Status),
			Error:  hs.Error,
		}
		if hs.Snapshot != nil {
			input.Sessions = ConvertProtoSessions(hs.Snapshot.Sessions)
			input.Groups = ConvertProtoGroups(hs.Snapshot.Groups)
			uploadEvents = append(uploadEvents, hs.Snapshot.UploadEvents...)
		}
		inputs = append(inputs, input)
	}
	return MultiHostSnapshotMsg{Snapshots: inputs, UploadEvents: uploadEvents}
}

// HubSnapshotMsg 是從 hub daemon 接收的多主機聚合快照。
type HubSnapshotMsg struct {
	Snapshot *tsmv1.MultiHostSnapshot
	Err      error
}

// isHubUnavailable 判斷錯誤是否為 daemon 不支援 hub 模式的永久性錯誤。
func isHubUnavailable(err error) bool {
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.Unavailable && st.Message() == "not in hub mode"
	}
	return false
}

// watchHubCmd 從已建立的 WatchMultiHost stream 接收第一個快照。
// 呼叫前必須已在 main.go 中呼叫 c.WatchMultiHost() 建立 stream。
func watchHubCmd(c *client.Client) tea.Cmd {
	return recvHubSnapshotCmd(c)
}

// recvHubSnapshotCmd 從 WatchMultiHost stream 接收下一個快照。
func recvHubSnapshotCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		snap, err := c.RecvMultiHostSnapshot()
		if err != nil {
			return HubSnapshotMsg{Err: err}
		}
		return HubSnapshotMsg{Snapshot: snap}
	}
}

// clientForCursor 回傳目前游標所在主機的 gRPC 客戶端。
// 多主機模式下依 HostID 路由，否則回傳 Deps.Client。
func (m Model) clientForCursor() *client.Client {
	if m.deps.HostMgr != nil && m.cursor >= 0 && m.cursor < len(m.items) {
		return m.deps.HostMgr.ClientFor(m.items[m.cursor].HostID)
	}
	return m.deps.Client
}

// clientForHost 回傳指定主機的 gRPC 客戶端。
// hostID 為空時回傳 Deps.Client（本機或單一 daemon）。
func (m Model) clientForHost(hostID string) *client.Client {
	if hostID != "" && m.deps.HostMgr != nil {
		return m.deps.HostMgr.ClientFor(hostID)
	}
	return m.deps.Client
}

// hostForCursor 回傳目前游標所在項目的主機 ID（用於 attach 路由）。
func (m Model) hostForCursor() string {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return m.items[m.cursor].HostID
	}
	return ""
}

// SelectedItem 回傳被選取的 ListItem（供 main.go 判斷 attach 目標主機）。
// 多主機模式下會同時比對 HostID，避免跨主機同名 session 誤判。
// 若 session 不在 items 中（例如剛建立），依 selectedHostID 建立 fallback。
func (m Model) SelectedItem() ListItem {
	for _, item := range m.items {
		if item.Type != ItemSession || item.Session.Name != m.selected {
			continue
		}
		// 多主機模式：必須 HostID 也匹配
		if m.selectedHostID != "" && item.HostID != m.selectedHostID {
			continue
		}
		return item
	}
	// Fallback：session 尚未出現在 items 中（剛建立的 session）
	if m.selected != "" && m.selectedHostID != "" {
		return ListItem{
			Type:   ItemSession,
			HostID: m.selectedHostID,
			Session: tmux.Session{Name: m.selected},
		}
	}
	return ListItem{}
}

// DetectSessionStatusResult 封裝直接模式的狀態偵測結果。
type DetectSessionStatusResult struct {
	Status tmux.SessionStatus
	AiType string
}

// detectSessionStatus 整合三層狀態偵測，回傳 session 的實際狀態和 AI presence。
func detectSessionStatus(deps Deps, sessionName, paneTitle, paneContent string) DetectSessionStatusResult {
	var input tmux.StatusInput
	var aiType string
	hookFound := false // 是否有找到 hook 狀態檔（區分「無 hook」與「hook 說不是 AI」）

	// 第一層：Hook 狀態檔案
	if deps.StatusDir != "" {
		if hs, err := tmux.ReadHookStatus(deps.StatusDir, sessionName); err == nil {
			input.HookStatus = &hs
			aiType = hs.AiType
			hookFound = true
		}
	}

	// 第二層（pane title）+ 第三層（terminal content）：只在 hook 無效時執行
	if input.HookStatus == nil || !input.HookStatus.IsValid() {
		input.PaneTitle = paneTitle
		input.PaneContent = paneContent
	}

	// ai_type 過期驗證
	if aiType != "" && (input.HookStatus == nil || !input.HookStatus.IsValid()) {
		if !ai.HasStrongAiPresence(paneContent) {
			aiType = ""
		}
	}

	// 降級偵測：無 hook 時從 pane content 偵測 AI 工具。
	// 有效 hook 且 ai_type 為空 → hook 已明確表示非 AI session，不降級。
	hookAuthoritative := hookFound && input.HookStatus != nil && input.HookStatus.IsValid()
	if aiType == "" && !hookAuthoritative && paneContent != "" {
		if tool := ai.DetectTool(paneContent); tool != "" {
			aiType = tool
		}
	}

	return DetectSessionStatusResult{
		Status: tmux.ResolveStatus(input),
		AiType: aiType,
	}
}

// pollInterval 回傳設定的輪詢間隔，若未設定則預設 2 秒。
func (m Model) pollInterval() time.Duration {
	interval := time.Duration(m.deps.Cfg.PollIntervalSec) * time.Second
	if interval <= 0 {
		return 2 * time.Second
	}
	return interval
}

// Init 實作 tea.Model 介面。
func (m Model) Init() tea.Cmd {
	if m.deps.HubMode && m.deps.Client != nil {
		// Hub 模式：透過 WatchMultiHost stream 接收多主機聚合快照
		return watchHubCmd(m.deps.Client)
	}
	if m.deps.HostMgr != nil {
		// 多主機模式（主要路徑）：立即送出目前快照，再等待後續通知
		mgr := m.deps.HostMgr
		return tea.Batch(
			func() tea.Msg { return buildMultiHostMsg(mgr) },
			recvMultiHostCmd(mgr),
		)
	}
	if m.deps.Client != nil {
		// 單一遠端模式（runRemote 使用）：啟動 Watch stream
		return watchCmd(m.deps.Client)
	}
	// 舊模式：直接輪詢（保留給測試用）
	return tea.Batch(
		loadSessionsCmd(m.deps),
		tickCmd(m.pollInterval()),
	)
}

// Update 處理訊息並更新模型狀態。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case SnapshotMsg:
		if msg.Err != nil {
			m.err = msg.Err
			// Remote 模式：Watch 錯誤代表 tunnel 已斷，退出讓 runRemote 處理重連
			if m.deps.RemoteMode {
				m.watchFailed = true
				m.quitting = true
				return m, tea.Quit
			}
			// 本地模式：延遲 2 秒重連 Watch stream
			if m.deps.Client != nil {
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return reconnectMsg{}
				})
			}
			return m, nil
		}
		sessions := ConvertProtoSessions(msg.Snapshot.Sessions)
		groups := ConvertProtoGroups(msg.Snapshot.Groups)
		m.items = FlattenItems(groups, sessions)
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		// 上傳模式：處理 upload events
		if m.mode == ModeUpload && msg.Snapshot.UploadEvents != nil {
			for _, event := range msg.Snapshot.UploadEvents {
				m.uploadModal.addEvent(event)
			}
		}
		// 繼續接收下一個快照；若有 running session 則啟動動畫 tick
		var cmds []tea.Cmd
		if m.deps.Client != nil {
			cmds = append(cmds, recvSnapshotCmd(m.deps.Client))
		}
		if m.hasRunning() {
			cmds = append(cmds, animTickCmd())
		}
		return m, tea.Batch(cmds...)
	case HubSnapshotMsg:
		if msg.Err != nil {
			// daemon 不再支援 hub 模式（永久性錯誤）→ 降級到 HostManager
			if isHubUnavailable(msg.Err) {
				m.hubDegraded = true
				return m, tea.Quit
			}
			m.err = msg.Err
			if m.deps.RemoteMode {
				return m, tea.Quit
			}
			// 延遲 2 秒重連 WatchMultiHost stream
			if m.deps.Client != nil {
				return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
					return hubReconnectMsg{}
				})
			}
			return m, nil
		}
		m.hubHostSnap = msg.Snapshot
		inputs := ConvertMultiHostSnapshot(msg.Snapshot)
		inputs = FilterActiveHosts(inputs, m.deps.Cfg.Hosts)
		m.items = FlattenMultiHost(inputs)
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		var cmds []tea.Cmd
		if m.deps.Client != nil {
			cmds = append(cmds, recvHubSnapshotCmd(m.deps.Client))
		}
		if m.hasRunning() {
			cmds = append(cmds, animTickCmd())
		}
		return m, tea.Batch(cmds...)
	case MultiHostSnapshotMsg:
		filtered := FilterActiveHosts(msg.Snapshots, m.deps.Cfg.Hosts)
		m.items = FlattenMultiHost(filtered)
		if m.cursor >= len(m.items) && len(m.items) > 0 {
			m.cursor = len(m.items) - 1
		}
		// 上傳模式：處理 upload events
		if m.mode == ModeUpload && len(msg.UploadEvents) > 0 {
			for _, event := range msg.UploadEvents {
				m.uploadModal.addEvent(event)
			}
		}
		var cmds []tea.Cmd
		cmds = append(cmds, recvMultiHostCmd(m.deps.HostMgr))
		if m.hasRunning() {
			cmds = append(cmds, animTickCmd())
		}
		return m, tea.Batch(cmds...)
	case SessionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		cursorName, cursorIsGroup := m.cursorItemKey()
		m.items = FlattenItems(msg.Groups, msg.Sessions)
		m.restoreCursor(cursorName, cursorIsGroup)
		if m.hasRunning() {
			return m, animTickCmd()
		}
		return m, nil
	case reconnectMsg:
		if m.deps.Client != nil {
			m.err = nil
			return m, watchCmd(m.deps.Client)
		}
		return m, nil
	case hubReconnectMsg:
		if m.deps.Client != nil {
			m.err = nil
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := m.deps.Client.WatchMultiHost(ctx); err != nil {
					return HubSnapshotMsg{Err: err}
				}
				return recvHubSnapshotCmd(m.deps.Client)()
			}
		}
		return m, nil
	case errMsg:
		m.err = msg.err
		return m, nil
	case AnimTickMsg:
		m.animFrame++
		if m.hasRunning() {
			return m, animTickCmd()
		}
		return m, nil
	case TickMsg:
		return m, tea.Batch(loadSessionsCmd(m.deps), tickCmd(m.pollInterval()))
	case CheckUpgradeMsg:
		m.upgradeChecking = false
		if msg.Err != nil {
			// GitHub 檢查失敗，但仍可偵測本機已安裝的較新版本
			if msg.InstalledVer != "" && upgrade.NeedsUpgrade(version.Version, msg.InstalledVer) {
				return m.offerRestart(msg.InstalledVer, msg.InstalledPath), nil
			}
			m.mode = ModeConfirm
			m.confirmPrompt = fmt.Sprintf("檢查失敗：%v", msg.Err)
			return m, nil
		}
		// 多主機模式：進入升級面板
		if m.deps.HostMgr != nil && msg.HostVersions != nil && msg.Release != nil {
			return m.enterModeUpgrade(msg.Release.Version, msg.HostVersions), nil
		}
		if !upgrade.NeedsUpgrade(version.Version, msg.Release.Version) {
			// GitHub 已是最新，但磁碟上的 binary 可能已由其他 tab 更新
			if msg.InstalledVer != "" && upgrade.NeedsUpgrade(version.Version, msg.InstalledVer) {
				return m.offerRestart(msg.InstalledVer, msg.InstalledPath), nil
			}
			m.mode = ModeConfirm
			m.confirmPrompt = fmt.Sprintf("已是最新版本 (%s)", version.Version)
			return m, nil
		}
		asset := upgrade.AssetName()
		url, ok := msg.Release.Assets[asset]
		if !ok {
			m.mode = ModeConfirm
			m.confirmPrompt = fmt.Sprintf("找不到適用於此平台的檔案 (%s)", asset)
			return m, nil
		}
		m.upgradeVersion = msg.Release.Version
		m.mode = ModeConfirm
		m.confirmPrompt = fmt.Sprintf("v%s → v%s 升級？", version.Version, msg.Release.Version)
		u := m.deps.Upgrader
		m.confirmAction = func() tea.Cmd {
			return downloadUpgradeCmd(u, url)
		}
		return m, nil
	case DownloadUpgradeMsg:
		if msg.Err != nil {
			if m.mode == ModeUpgrade {
				// 在升級面板中，更新 local 項目為失敗
				for i := range m.upgradeItems {
					if m.upgradeItems[i].IsLocal {
						m.upgradeItems[i].Status = UpgradeFailed
						m.upgradeItems[i].Error = msg.Err.Error()
						m.upgradeItems[i].Checked = true
						break
					}
				}
				m.upgradeRunning = false
				return m, nil
			}
			m.mode = ModeConfirm
			m.confirmPrompt = fmt.Sprintf("下載失敗：%v", msg.Err)
			return m, nil
		}
		m.upgradeTmpPath = msg.TmpPath
		m.upgradeReady = true
		m.quitting = true
		return m, tea.Quit
	case RemoteUpgradeMsg:
		m, cmd := m.handleRemoteUpgrade(msg)
		return m, cmd
	case tea.KeyMsg:
		switch m.mode {
		case ModeInput:
			return m.updateInput(msg)
		case ModeConfirm:
			return m.updateConfirm(msg)
		case ModePicker:
			return m.updatePicker(msg)
		case ModeSearch:
			return m.updateSearch(msg)
		case ModeHostPicker:
			return m.updateHostPicker(msg)
		case ModeNewSession:
			return m.updateNewSession(msg)
		case ModeUpload:
			return m.updateUpload(msg)
		case ModeUpgrade:
			return m.updateUpgrade(msg)
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

// updateNormal 處理一般瀏覽模式的按鍵。
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "ctrl+e":
		m.quitting = true
		m.exitTmux_ = true
		return m, tea.Quit
	case "j", "down":
		if len(m.items) > 0 {
			m.cursor = (m.cursor + 1) % len(m.items)
		}
		return m, nil
	case "k", "up":
		if len(m.items) > 0 {
			m.cursor = (m.cursor - 1 + len(m.items)) % len(m.items)
		}
		return m, nil
	case "enter":
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			switch item.Type {
			case ItemHostTitle:
				return m, nil // 主機標題列不可操作
			case ItemSession:
				m.selected = item.Session.Name
				m.selectedHostID = item.HostID
				m.quitting = true
				return m, tea.Quit
			case ItemGroup:
				return m.toggleCollapse(item)
			}
		}
		return m, nil
	case "tab":
		if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
			return m.toggleCollapse(m.items[m.cursor])
		}
		return m, nil
	case "left", "right":
		// 群組上切換展開/收合
		if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
			return m.toggleCollapse(m.items[m.cursor])
		}
		return m, nil
	case "c":
		m.openConfig_ = true
		m.quitting = true
		return m, tea.Quit
	case "n":
		var recentPaths []string
		if m.deps.Store != nil {
			recentPaths, _ = m.deps.Store.RecentPaths(3)
		}
		// 建立可選主機清單（多主機模式或 hub 模式）
		var hosts []hostTabInfo
		if m.deps.HostMgr != nil {
			for _, h := range m.deps.HostMgr.Hosts() {
				cfg := h.Config()
				if cfg.Enabled {
					hosts = append(hosts, hostTabInfo{ID: cfg.Name, Name: cfg.Name, Color: cfg.Color})
				}
			}
		} else if m.deps.HubMode && m.hubHostSnap != nil {
			for _, h := range m.hubHostSnap.Hosts {
				if h.Status != tsmv1.HostStatus_HOST_STATUS_CONNECTED {
					continue // 只顯示已連線的主機，避免使用者選到不可用的目標
				}
				hosts = append(hosts, hostTabInfo{
					ID: h.HostId, Name: h.Name, Color: h.Color,
				})
			}
		}
		m.newSession.initForm(m.deps.Cfg.Agents, recentPaths, hosts)
		m.mode = ModeNewSession
		return m, nil
	case "r":
		// 重命名：session 雙行輸入（名稱+ID），群組單行重命名
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Type == ItemHostTitle {
				return m, nil // 主機標題列不可操作
			}
			switch item.Type {
			case ItemSession:
				m.mode = ModeInput
				m.inputTarget = InputRenameSession
				m.inputRow = 0
				m.textInputs[0].SetValue(item.Session.CustomName)
				m.textInputs[0].Focus()
				m.textInputs[1].SetValue(item.Session.Name)
				m.textInputs[1].Blur()
			case ItemGroup:
				m.mode = ModeInput
				m.inputTarget = InputRenameGroup
				m.inputPrompt = "群組名稱"
				m.textInput.SetValue(item.Group.Name)
				m.textInput.Focus()
			}
		}
		return m, nil
	case "g":
		// 新建群組：需要 Store、Client 或 HostMgr
		if m.clientForCursor() != nil || m.deps.Store != nil {
			m.mode = ModeInput
			m.inputTarget = InputNewGroup
			m.inputPrompt = "群組名稱"
			m.textInput.SetValue("")
			m.textInput.Focus()
		}
		return m, nil
	case "m":
		// 移動 session 到群組：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) && (m.clientForCursor() != nil || m.deps.Store != nil) {
			item := m.items[m.cursor]
			if item.Type == ItemHostTitle {
				return m, nil // 主機標題列不可操作
			}
			if item.Type == ItemSession {
				groups := m.collectGroups()
				if len(groups) == 0 {
					return m, nil
				}
				m.mode = ModePicker
				m.pickerGroups = groups
				m.pickerCursor = 0
				m.pickerTarget = item.Session.Name
			}
		}
		return m, nil
	case "h":
		// 主機管理面板：多主機模式或 hub 模式有效
		if m.deps.HostMgr != nil || (m.deps.HubMode && m.hubHostSnap != nil) {
			m.mode = ModeHostPicker
			m.hostPickerCursor = 0
			return m, nil
		}
		return m, nil
	case "/":
		// 進入搜尋模式
		m.mode = ModeSearch
		m.searchQuery = ""
		m.cursor = 0
		return m, nil
	case "R":
		// 唯讀進入 session
		if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemSession {
			m.readOnly_ = true
			m.selected = m.items[m.cursor].Session.Name
			m.selectedHostID = m.items[m.cursor].HostID
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case "J", "shift+down":
		return m.moveItem(1) // 下移
	case "K", "shift+up":
		return m.moveItem(-1) // 上移
	case "ctrl+u":
		if m.deps.Upgrader == nil || m.upgradeChecking {
			return m, nil
		}
		m.upgradeChecking = true
		return m, checkUpgradeCmd(m.deps.Upgrader, m.deps.HostMgr)
	case "U":
		// Shift+U：上傳模式（需有 gRPC 連線）
		if m.clientForCursor() == nil {
			return m, nil
		}
		if m.cursor < 0 || m.cursor >= len(m.items) || m.items[m.cursor].Type != ItemSession {
			return m, nil
		}
		item := m.items[m.cursor]
		m.mode = ModeUpload
		m.uploadModal = newUploadModal(item.Session.Name, item.HostID, item.Session.Path)
		return m, m.setUploadModeCmd(true, item.Session.Name)
	case "d":
		// 刪除 session：僅對 ItemSession 生效
		if m.cursor >= 0 && m.cursor < len(m.items) {
			item := m.items[m.cursor]
			if item.Type == ItemHostTitle {
				return m, nil // 主機標題列不可操作
			}
			if item.Type == ItemSession {
				name := item.Session.Name
				c := m.clientForCursor() // 閉包前取得當前游標的 client
				deps := m.deps
				hostID := m.hostForCursor()
				m.mode = ModeConfirm
				m.confirmPrompt = fmt.Sprintf("確定要刪除 session %q？", name)
				m.confirmAction = func() tea.Cmd {
					if c != nil {
						var err error
						if deps.HubMode {
							err = c.ProxyMutation(context.Background(), hostID,
								tsmv1.MutationType_MUTATION_KILL_SESSION, name, "", "", "", "")
						} else {
							err = c.KillSession(context.Background(), name)
						}
						if err != nil {
							return func() tea.Msg { return errMsg{err} }
						}
						return nil // 變更透過 Watch/MultiHost stream 自動推送
					}
					if deps.TmuxMgr != nil {
						if err := deps.TmuxMgr.KillSession(name); err != nil {
							return func() tea.Msg { return errMsg{err} }
						}
					}
					return loadSessionsCmd(deps)
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// updateInput 處理文字輸入模式的按鍵。
func (m Model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// InputRenameSession 使用雙行輸入，特殊處理
	if m.inputTarget == InputRenameSession {
		return m.updateDualInput(msg)
	}

	switch msg.Type {
	case tea.KeyEsc:
		// 取消輸入，回到一般模式
		m.mode = ModeNormal
		m.textInput.SetValue("")
		m.textInput.Blur()
		m.inputPrompt = ""
		return m, nil
	case tea.KeyEnter:
		value := strings.TrimSpace(m.textInput.Value())
		m.textInput.SetValue("")
		m.textInput.Blur()
		m.inputPrompt = ""
		if value == "" {
			m.mode = ModeNormal
			return m, nil
		}
		switch m.inputTarget {
		case InputNewGroup:
			m.mode = ModeNormal
			if c := m.clientForCursor(); c != nil {
				if err := c.CreateGroup(context.Background(), value, 0); err != nil {
					m.err = err
					return m, nil
				}
				return m, nil // 變更透過 Watch/MultiHost stream 自動推送
			}
			if m.deps.Store != nil {
				if err := m.deps.Store.CreateGroup(value, 0); err != nil {
					m.err = err
					return m, nil
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputRenameGroup:
			m.mode = ModeNormal
			if m.cursor >= 0 && m.cursor < len(m.items) && m.items[m.cursor].Type == ItemGroup {
				groupID := m.items[m.cursor].Group.ID
				if c := m.clientForCursor(); c != nil {
					if err := c.RenameGroup(context.Background(), groupID, value); err != nil {
						m.err = err
						return m, nil
					}
					return m, nil // 變更透過 Watch/MultiHost stream 自動推送
				}
				if m.deps.Store != nil {
					if err := m.deps.Store.RenameGroup(groupID, value); err != nil {
						m.err = err
						return m, nil
					}
				}
			}
			return m, loadSessionsCmd(m.deps)
		case InputNewHost:
			// 新增主機
			if m.deps.HostMgr != nil {
				// 檢查是否有同名封存主機 → 解封存
				if found, archivedHost := m.deps.HostMgr.FindArchived(value); found {
					archivedHost.SetArchived(false)
					_ = m.deps.HostMgr.Enable(context.Background(), archivedHost.ID())
					m.persistHosts()
					m.mode = ModeHostPicker
					return m, nil
				}
				hosts := m.deps.HostMgr.Hosts()
				usedColors := make(map[string]bool, len(hosts))
				for _, h := range hosts {
					usedColors[h.Config().Color] = true
				}
				color := config.DefaultColors[len(hosts)%len(config.DefaultColors)]
				for _, c := range config.DefaultColors {
					if !usedColors[c] {
						color = c
						break
					}
				}
				entry := config.HostEntry{
					Name:      value,
					Address:   value,
					Color:     color,
					Enabled:   true,
					SortOrder: len(hosts),
				}
				m.deps.HostMgr.AddHost(entry)
				_ = m.deps.HostMgr.Enable(context.Background(), value)
				m.persistHosts()
			}
			m.mode = ModeHostPicker // 回到主機管理面板
			return m, nil
		default:
			m.mode = ModeNormal
		}
		return m, nil
	default:
		// 委託給 textinput 處理所有其他按鍵（含空格、方向鍵、Backspace 等）
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

// updateDualInput 處理 InputRenameSession 的雙行輸入按鍵。
func (m Model) updateDualInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = ModeNormal
		m.textInputs[0].SetValue("")
		m.textInputs[0].Blur()
		m.textInputs[1].SetValue("")
		m.textInputs[1].Blur()
		return m, nil
	case tea.KeyUp:
		if m.inputRow > 0 {
			m.inputRow = 0
			m.textInputs[0].Focus()
			m.textInputs[1].Blur()
		}
		return m, nil
	case tea.KeyDown:
		if m.inputRow < 1 {
			m.inputRow = 1
			m.textInputs[1].Focus()
			m.textInputs[0].Blur()
		}
		return m, nil
	case tea.KeyEnter:
		if m.cursor < 0 || m.cursor >= len(m.items) {
			m.mode = ModeNormal
			return m, nil
		}
		origName := m.items[m.cursor].Session.Name
		customName := m.textInputs[0].Value() // 允許空（清除自訂名稱）
		newID := strings.TrimSpace(m.textInputs[1].Value())
		if newID == "" {
			newID = origName // ID 留空 = 不變更
		}

		m.textInputs[0].SetValue("")
		m.textInputs[0].Blur()
		m.textInputs[1].SetValue("")
		m.textInputs[1].Blur()
		m.mode = ModeNormal

		newSessionName := ""
		if newID != origName {
			newSessionName = newID
		}

		if c := m.clientForCursor(); c != nil {
			var err error
			if m.deps.HubMode {
				err = c.ProxyMutation(context.Background(), m.hostForCursor(),
					tsmv1.MutationType_MUTATION_RENAME_SESSION, origName, customName, "", "", "")
			} else {
				err = c.RenameSession(context.Background(), origName, customName, newSessionName)
			}
			if err != nil {
				m.err = err
				return m, nil
			}
			return m, nil // 變更透過 Watch/MultiHost stream 自動推送
		}
		// 舊模式：先 tmux rename，再 store 遷移，再設 custom name
		if newSessionName != "" && m.deps.TmuxMgr != nil {
			if err := m.deps.TmuxMgr.RenameSession(origName, newSessionName); err != nil {
				m.err = err
				return m, nil
			}
			if m.deps.Store != nil {
				if err := m.deps.Store.RenameSessionKey(origName, newSessionName); err != nil {
					m.err = err
					return m, nil
				}
			}
		}
		storeName := origName
		if newSessionName != "" {
			storeName = newSessionName
		}
		if m.deps.Store != nil {
			if err := m.deps.Store.SetCustomName(storeName, customName); err != nil {
				m.err = err
				return m, nil
			}
		}
		// 同步 tmux user option 供 status bar 原生格式顯示
		// 使用 session ID（$N）避免純數字 session name 被 tmux 誤解為 window index
		if m.deps.TmuxMgr != nil {
			target := m.items[m.cursor].Session.ID
			if target == "" {
				target = storeName
			}
			if customName != "" {
				_ = m.deps.TmuxMgr.SetSessionOption(target, "@tsm_name", customName)
			} else {
				_ = m.deps.TmuxMgr.UnsetSessionOption(target, "@tsm_name")
			}
		}
		return m, loadSessionsCmd(m.deps)
	default:
		var cmd tea.Cmd
		m.textInputs[m.inputRow], cmd = m.textInputs[m.inputRow].Update(msg)
		return m, cmd
	}
}

// updateConfirm 處理確認對話框模式的按鍵。
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	returnMode := m.confirmReturnMode // 預設 ModeNormal (零值)

	switch msg.String() {
	case "y", "Y", "enter":
		// 執行確認動作
		var cmd tea.Cmd
		if m.confirmAction != nil {
			cmd = m.confirmAction()
			m.confirmAction = nil
		}
		if m.confirmExitTmux {
			m.exitTmux_ = true
			m.quitting = true
			m.confirmExitTmux = false
		}
		m.mode = returnMode
		m.confirmPrompt = ""
		m.confirmReturnMode = ModeNormal
		return m, cmd
	case "n", "N", "esc", "q":
		// 取消確認
		m.mode = returnMode
		m.confirmPrompt = ""
		m.confirmAction = nil
		m.confirmExitTmux = false
		m.confirmReturnMode = ModeNormal
		return m, nil
	}
	return m, nil
}

// updatePicker 處理群組選擇器模式的按鍵。
func (m Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.pickerCursor < len(m.pickerGroups)-1 {
			m.pickerCursor++
		}
	case "k", "up":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case "enter":
		if m.pickerCursor >= 0 && m.pickerCursor < len(m.pickerGroups) {
			group := m.pickerGroups[m.pickerCursor]
			if c := m.clientForCursor(); c != nil {
				var err error
				if m.deps.HubMode {
					err = c.ProxyMutation(context.Background(), m.hostForCursor(),
						tsmv1.MutationType_MUTATION_MOVE_SESSION, m.pickerTarget, "",
						fmt.Sprintf("%d", group.ID), "", "")
				} else {
					err = c.MoveSession(context.Background(), m.pickerTarget, group.ID, 0)
				}
				if err != nil {
					m.err = err
				}
			} else if m.deps.Store != nil {
				if err := m.deps.Store.SetSessionGroup(m.pickerTarget, group.ID, 0); err != nil {
					m.err = err
				}
			}
		}
		m.mode = ModeNormal
		m.pickerGroups = nil
		m.pickerTarget = ""
		if m.clientForCursor() != nil {
			return m, nil // 變更透過 Watch/MultiHost stream 自動推送
		}
		return m, loadSessionsCmd(m.deps)
	case "esc":
		m.mode = ModeNormal
		m.pickerGroups = nil
		m.pickerTarget = ""
	}
	return m, nil
}

// updateSearch 處理搜尋模式的按鍵。
func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.mode = ModeNormal
		m.searchQuery = ""
		m.cursor = 0
	case tea.KeyEnter:
		visible := m.visibleItems()
		if m.cursor >= 0 && m.cursor < len(visible) {
			item := visible[m.cursor]
			if item.Type == ItemSession {
				m.selected = item.Session.Name
				m.selectedHostID = item.HostID
				m.quitting = true
				return m, tea.Quit
			}
		}
	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
		}
		visible := m.visibleItems()
		if m.cursor >= len(visible) && len(visible) > 0 {
			m.cursor = len(visible) - 1
		}
	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.cursor = 0
	default:
		// j/k 導航
		switch msg.String() {
		case "j", "down":
			visible := m.visibleItems()
			if m.cursor < len(visible)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		}
	}
	return m, nil
}

// toggleCollapse 切換群組的展開/收合狀態。
func (m Model) toggleCollapse(item ListItem) (tea.Model, tea.Cmd) {
	if c := m.clientForCursor(); c != nil {
		if err := c.ToggleCollapse(context.Background(), item.Group.ID); err != nil {
			m.err = err
		}
		return m, nil // 變更透過 Watch/MultiHost stream 自動推送
	}
	if m.deps.Store != nil {
		if err := m.deps.Store.ToggleGroupCollapsed(item.Group.ID); err != nil {
			m.err = err
			return m, nil
		}
		return m, loadSessionsCmd(m.deps)
	}
	return m, nil
}

// collectGroups 從目前的 items 中收集所有群組。
// 多主機模式下只收集游標所在主機的群組。
func (m Model) collectGroups() []store.Group {
	if m.deps.Store != nil {
		groups, _ := m.deps.Store.ListGroups()
		return groups
	}
	hostID := m.hostForCursor()
	var groups []store.Group
	for _, item := range m.items {
		if item.Type == ItemGroup {
			if hostID != "" && item.HostID != hostID {
				continue
			}
			groups = append(groups, item.Group)
		}
	}
	return groups
}

// moveItem 根據方向移動目前選取的項目（群組或 session）。
func (m Model) moveItem(direction int) (tea.Model, tea.Cmd) {
	if m.clientForCursor() == nil && m.deps.Store == nil {
		return m, nil
	}
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return m, nil
	}
	item := m.items[m.cursor]
	if item.Type == ItemHostTitle {
		return m, nil // 主機標題列不可操作
	}
	switch item.Type {
	case ItemGroup:
		return m.moveGroup(item.Group, direction)
	case ItemSession:
		return m.moveSession(item.Session, direction)
	}
	return m, nil
}

// normalizeGroupOrders 當群組的 sort_order 有重複時，重新指派為連續值。
func (m Model) normalizeGroupOrders(groups []store.Group) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, g := range groups {
		if seen[g.SortOrder] {
			needsNormalize = true
			break
		}
		seen[g.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	c := m.clientForCursor()
	for i, g := range groups {
		if c != nil {
			if err := c.ReorderGroup(context.Background(), g.ID, i); err != nil {
				return err
			}
		} else if m.deps.Store != nil {
			if err := m.deps.Store.SetGroupOrder(g.ID, i); err != nil {
				return err
			}
		}
		groups[i].SortOrder = i
	}
	return nil
}

// moveGroup 將群組在排序中上移或下移，交換 sort_order。
func (m Model) moveGroup(group store.Group, direction int) (tea.Model, tea.Cmd) {
	groups := m.collectGroups()
	idx := -1
	for i, g := range groups {
		if g.ID == group.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m, nil
	}
	newIdx := (idx + direction + len(groups)) % len(groups)
	if newIdx == idx {
		return m, nil
	}

	// sort_order 重複時先正規化
	if err := m.normalizeGroupOrders(groups); err != nil {
		m.err = err
		return m, nil
	}

	// 找到目標群組在 items 中的位置（交換後 cursor 應跟隨到此處）
	targetID := groups[newIdx].ID
	newCursor := m.cursor
	for i, item := range m.items {
		if item.Type == ItemGroup && item.Group.ID == targetID {
			newCursor = i
			break
		}
	}

	if c := m.clientForCursor(); c != nil {
		// gRPC 模式：交換 sort_order
		if err := c.ReorderGroup(context.Background(), groups[idx].ID, groups[newIdx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		if err := c.ReorderGroup(context.Background(), groups[newIdx].ID, groups[idx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		m.cursor = newCursor
		return m, nil // 變更透過 Watch/MultiHost stream 自動推送
	}

	// 舊模式
	if err := m.deps.Store.SetGroupOrder(groups[idx].ID, groups[newIdx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	if err := m.deps.Store.SetGroupOrder(groups[newIdx].ID, groups[idx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	m.cursor = newCursor
	return m, loadSessionsCmd(m.deps)
}

// normalizeSessionOrders 當同群組 session 的 sort_order 有重複時，重新指派為連續值。
// gRPC 模式使用 siblings（來自 items），舊模式使用 metas（來自 store）。
func (m Model) normalizeSessionOrders(siblings []tmux.Session, groupID int64) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, s := range siblings {
		if seen[s.SortOrder] {
			needsNormalize = true
			break
		}
		seen[s.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	c := m.clientForCursor()
	for i, s := range siblings {
		if c != nil {
			if err := c.ReorderSession(context.Background(), s.Name, groupID, i); err != nil {
				return err
			}
		} else if m.deps.Store != nil {
			if err := m.deps.Store.SetSessionGroup(s.Name, groupID, i); err != nil {
				return err
			}
		}
		siblings[i].SortOrder = i
	}
	return nil
}

// normalizeMetaOrders 當 session metas 的 sort_order 有重複時，重新指派為連續值（舊模式）。
func (m Model) normalizeMetaOrders(metas []store.SessionMeta, groupID int64) error {
	seen := make(map[int]bool)
	needsNormalize := false
	for _, meta := range metas {
		if seen[meta.SortOrder] {
			needsNormalize = true
			break
		}
		seen[meta.SortOrder] = true
	}
	if !needsNormalize {
		return nil
	}
	for i, meta := range metas {
		if err := m.deps.Store.SetSessionGroup(meta.SessionName, groupID, i); err != nil {
			return err
		}
		metas[i].SortOrder = i
	}
	return nil
}

// moveSession 將 session 在群組內（或未分組）排序中上移或下移，交換 sort_order。
func (m Model) moveSession(session tmux.Session, direction int) (tea.Model, tea.Cmd) {
	var groupID int64
	if session.GroupName == "" {
		groupID = 0
	} else {
		groups := m.collectGroups()
		for _, g := range groups {
			if g.Name == session.GroupName {
				groupID = g.ID
				break
			}
		}
		if groupID == 0 {
			return m, nil
		}
	}

	if c := m.clientForCursor(); c != nil {
		// gRPC 模式：需要從 items 中找同群組的 session 來計算排序
		var siblings []tmux.Session
		for _, item := range m.items {
			if item.Type == ItemSession && item.Session.GroupName == session.GroupName {
				siblings = append(siblings, item.Session)
			}
		}
		idx := -1
		for i, s := range siblings {
			if s.Name == session.Name {
				idx = i
				break
			}
		}
		if idx < 0 {
			return m, nil
		}
		newIdx := (idx + direction + len(siblings)) % len(siblings)
		if newIdx == idx {
			return m, nil
		}
		// sort_order 重複時先正規化
		if err := m.normalizeSessionOrders(siblings, groupID); err != nil {
			m.err = err
			return m, nil
		}
		if err := c.ReorderSession(context.Background(), siblings[idx].Name, groupID, siblings[newIdx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		if err := c.ReorderSession(context.Background(), siblings[newIdx].Name, groupID, siblings[idx].SortOrder); err != nil {
			m.err = err
			return m, nil
		}
		// 找到目標 session 在 items 中的位置
		targetName := siblings[newIdx].Name
		newCursor := m.cursor
		for i, item := range m.items {
			if item.Type == ItemSession && item.Session.Name == targetName {
				newCursor = i
				break
			}
		}
		m.items[m.cursor], m.items[newCursor] = m.items[newCursor], m.items[m.cursor]
		m.cursor = newCursor
		return m, nil // 變更透過 Watch/MultiHost stream 自動推送
	}

	// 舊模式
	metas, err := m.deps.Store.ListSessionMetas(groupID)
	if err != nil {
		m.err = err
		return m, nil
	}
	idx := -1
	for i, meta := range metas {
		if meta.SessionName == session.Name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m, nil
	}
	newIdx := (idx + direction + len(metas)) % len(metas)
	if newIdx == idx {
		return m, nil
	}
	// sort_order 重複時先正規化
	if err := m.normalizeMetaOrders(metas, groupID); err != nil {
		m.err = err
		return m, nil
	}
	// 交換 sort_order
	if err := m.deps.Store.SetSessionGroup(metas[idx].SessionName, groupID, metas[newIdx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	if err := m.deps.Store.SetSessionGroup(metas[newIdx].SessionName, groupID, metas[idx].SortOrder); err != nil {
		m.err = err
		return m, nil
	}
	// 找到目標 session 在 items 中的位置
	targetName := metas[newIdx].SessionName
	newCursor := m.cursor
	for i, item := range m.items {
		if item.Type == ItemSession && item.Session.Name == targetName {
			newCursor = i
			break
		}
	}
	m.cursor = newCursor
	return m, loadSessionsCmd(m.deps)
}

// cursorItemKey 回傳 cursor 目前指向的 item 識別資訊。
func (m Model) cursorItemKey() (name string, isGroup bool) {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		item := m.items[m.cursor]
		if item.Type == ItemSession {
			return item.Session.Name, false
		}
		if item.Type == ItemGroup {
			return item.Group.Name, true
		}
	}
	return "", false
}

// restoreCursor 嘗試將 cursor 還原到指定 item 位置。
func (m *Model) restoreCursor(name string, isGroup bool) {
	if name != "" {
		for i, item := range m.items {
			if !isGroup && item.Type == ItemSession && item.Session.Name == name {
				m.cursor = i
				return
			}
			if isGroup && item.Type == ItemGroup && item.Group.Name == name {
				m.cursor = i
				return
			}
		}
	}
	if m.cursor >= len(m.items) && len(m.items) > 0 {
		m.cursor = len(m.items) - 1
	}
}

// SetConfirm 設定確認模式（主要用於測試）。
func (m *Model) SetConfirm(prompt string, action func() tea.Cmd) {
	m.mode = ModeConfirm
	m.confirmPrompt = prompt
	m.confirmAction = action
}

// View 渲染 TUI 畫面。
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	title := headerStyle.Render("tmux session menu")
	mode := versionStyle.Render(" [" + m.connectionMode() + "]")
	ver := versionStyle.Render("  " + version.String())
	b.WriteString(title + mode + ver + "\n")

	// Items list
	displayItems := m.visibleItems()
	if len(displayItems) > 0 {
		b.WriteString("\n")

		// 計算群組行號總數（只有群組佔行號）以決定對齊位數
		groupCount := 0
		for _, item := range displayItems {
			if item.Type == ItemGroup {
				groupCount++
			}
		}
		maxDigits := 1
		if groupCount > 0 {
			maxDigits = len(fmt.Sprintf("%d", groupCount))
		}
		// 前綴寬度：數字位數 + 1（中點）
		prefixWidth := maxDigits + 1

		lineNum := 0
		for i, item := range displayItems {
			isGroupChild := item.Type == ItemSession && item.Session.GroupName != ""
			isCursor := i == m.cursor

			if item.Type == ItemGroup {
				lineNum++
			}

			switch item.Type {
			case ItemGroup:
				collapse := "▾"
				if item.Group.Collapsed {
					collapse = "▸"
				}

				numStr := fmt.Sprintf("%d", lineNum)
				padding := strings.Repeat(" ", maxDigits-len(numStr))

				var line string
				if isCursor {
					// cursor 行：數字+中點亮白，箭頭+名稱群組色，背景色
					line = fmt.Sprintf("%s%s%s %s",
						padding,
						sessionNameStyle.Render(numStr+"·"),
						groupNameStyle.Render(collapse),
						groupNameStyle.Render(item.Group.Name))
					line = m.cursorLine(line)
				} else {
					line = fmt.Sprintf("%s%s%s %s",
						padding,
						dimStyle.Render(numStr+"·"),
						groupNameStyle.Render(collapse),
						groupNameStyle.Render(item.Group.Name))
				}
				b.WriteString(line + "\n")

			case ItemSession:
				icon := item.Session.StatusIcon()
				styledIcon := m.styledStatusIcon(item.Session.Status, icon)

				summary := ""
				if item.Session.AISummary != "" {
					summary = "  " + summaryStyle.Render(item.Session.AISummary)
				}

				relTime := ""
				if !item.Session.Activity.IsZero() {
					relTime = "  " + dimStyle.Render(item.Session.RelativeTime())
				}

				aiModel := ""
				if item.Session.AIModel != "" {
					aiModel = "  " + dimStyle.Render(item.Session.AIModel)
				}

				name := item.Session.DisplayName()
				if isCursor {
					name = sessionNameStyle.Render(name)
				} else {
					name = sessionNameStyle.Render(name)
				}

				if isGroupChild {
					// 樹狀符號：判斷是否為群組最後一個子項目
					isLast := true
					if i+1 < len(displayItems) {
						next := displayItems[i+1]
						if next.Type == ItemSession && next.Session.GroupName == item.Session.GroupName {
							isLast = false
						}
					}
					tree := dimStyle.Render("├─")
					if isLast {
						tree = dimStyle.Render("└─")
					}
					prefix := strings.Repeat(" ", prefixWidth)
					line := fmt.Sprintf("%s%s %s %s%s%s%s",
						prefix, tree, styledIcon, name, summary, relTime, aiModel)
					if isCursor {
						line = m.cursorLine(line)
					}
					b.WriteString(line + "\n")
				} else {
					// 未分組 session：不顯示行號，但留白對齊
					prefix := strings.Repeat(" ", prefixWidth)
					line := fmt.Sprintf("%s%s %s%s%s%s",
						prefix, styledIcon, name, summary, relTime, aiModel)
					if isCursor {
						line = m.cursorLine(line)
					}
					b.WriteString(line + "\n")
				}

			case ItemHostTitle:
				// 主機標題 badge 風格
				titleStyle := lipgloss.NewStyle().
					Background(lipgloss.Color(item.HostColor)).
					Foreground(lipgloss.Color("#ffffff")).
					Padding(0, 1)

				if item.HostState != HostStateConnected {
					titleStyle = titleStyle.Background(lipgloss.Color("#555555"))
				}
				title := titleStyle.Render(item.HostID)

				// 非連線狀態：右邊加狀態文字
				if item.HostState != HostStateConnected {
					var stateText string
					switch item.HostState {
					case HostStateConnecting:
						stateText = "連線中..."
					case HostStateDisconnected:
						stateText = "連線中斷，重連中..."
					case HostStateDisabled:
						stateText = "已停用"
					}
					title += " " + dimStyle.Render(stateText)
				}

				if isCursor {
					title = m.cursorLine(title)
				}
				b.WriteString(title + "\n")
			}
		}
	}

	// Error display
	if m.err != nil {
		b.WriteString(fmt.Sprintf("\n  %s\n", statusErrorStyle.Render("Error: "+m.err.Error())))
	}

	// 模式相關的底部列
	switch m.mode {
	case ModeInput:
		if m.inputTarget == InputRenameSession {
			// 雙行輸入
			row0Marker := "  "
			row1Marker := "  "
			if m.inputRow == 0 {
				row0Marker = selectedStyle.Render("► ")
			} else {
				row1Marker = selectedStyle.Render("► ")
			}
			b.WriteString(fmt.Sprintf("\n  %s%s: %s\n",
				row0Marker,
				selectedStyle.Render("名稱"),
				m.textInputs[0].View()))
			b.WriteString(fmt.Sprintf("  %s%s: %s\n",
				row1Marker,
				selectedStyle.Render("ID"),
				m.textInputs[1].View()))
			b.WriteString(fmt.Sprintf("  %s\n",
				dimStyle.Render("[↑↓] 切換  [Enter] 確認  [Esc] 取消")))
		} else {
			b.WriteString(fmt.Sprintf("\n  %s: %s\n",
				selectedStyle.Render(m.inputPrompt),
				m.textInput.View()))
			b.WriteString(fmt.Sprintf("  %s\n",
				dimStyle.Render("[Esc] 取消")))
		}
	case ModeConfirm:
		b.WriteString(fmt.Sprintf("\n  %s  %s\n",
			selectedStyle.Render(m.confirmPrompt),
			dimStyle.Render("[y] 確認  [n/Esc] 取消")))
	case ModePicker:
		b.WriteString(fmt.Sprintf("\n  %s\n", selectedStyle.Render("移動到群組：")))
		for i, g := range m.pickerGroups {
			cursor := "  "
			if i == m.pickerCursor {
				cursor = selectedStyle.Render("► ")
			}
			b.WriteString(fmt.Sprintf("  %s%s\n", cursor, g.Name))
		}
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("[Enter] 確認  [Esc] 取消")))
	case ModeSearch:
		b.WriteString(fmt.Sprintf("\n  %s: %s█\n",
			selectedStyle.Render("/"),
			m.searchQuery))
		b.WriteString(fmt.Sprintf("  %s\n", dimStyle.Render("[Enter] 選擇  [Esc] 取消")))
	case ModeHostPicker:
		b.WriteString(m.renderHostPicker())
	case ModeNewSession:
		b.WriteString(m.renderNewSession())
	case ModeUpload:
		b.WriteString("\n")
		b.WriteString(m.uploadModal.view())
	case ModeUpgrade:
		b.WriteString(m.renderUpgrade())
	default:
		b.WriteString("\n")
		b.WriteString(m.renderToolbar())
	}

	output := b.String()
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if m.deps.Cfg.InPopup {
			lines[i] = "  " + line + " "
		} else {
			lines[i] = " " + line
		}
	}
	output = strings.Join(lines, "\n")
	return output
}

// renderToolbar 渲染底部工具列，固定兩行佈局。
func (m Model) renderToolbar() string {
	render := func(key, desc string) string {
		return keyStyle.Render(key) + versionStyle.Render(" "+desc)
	}
	line2Parts := []string{
		render("[m]", "搬移"), render("[⇧+↑/⇧+k]", "上移"), render("[⇧+↓/⇧+j]", "下移"), render("[c]", "設定"), render("[ctrl+e]", "退出"),
	}
	if m.deps.HostMgr != nil || m.deps.HubMode {
		line2Parts = append(line2Parts, render("[h]", "主機管理"))
	}
	if m.deps.HostMgr != nil || m.deps.Client != nil {
		line2Parts = append(line2Parts, render("[U]", "上傳"))
	}
	if m.deps.Upgrader != nil {
		line2Parts = append(line2Parts, render("[ctrl+u]", "升級"))
	}
	lines := []string{
		fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
			render("[/]", "搜尋"), render("[n]", "新建"), render("[r]", "更名"), render("[d]", "刪除"), render("[R]", "唯讀進入"), render("[q]", "關閉選單")),
		"  " + strings.Join(line2Parts, "  "),
	}
	return strings.Join(lines, "\n") + "\n"
}

// cursorLine 將行內容加上背景色，並用空格填充到終端寬度。
// lipgloss Render 會插入 \033[49m（背景重置）和 \033[0m（全重置），
// 必須將它們替換為 cursor 背景色，才能讓背景貫穿整行。
var cursorBgReplacer = strings.NewReplacer(
	"\033[49m", "\033[48;2;55;55;55m",
	"\033[0m", "\033[0m\033[48;2;55;55;55m",
)

func (m Model) cursorLine(line string) string {
	const bg = "\033[48;2;55;55;55m"
	line = cursorBgReplacer.Replace(line)
	visWidth := lipgloss.Width(line)
	pad := ""
	if m.width > visWidth {
		pad = strings.Repeat(" ", m.width-visWidth)
	}
	return bg + line + pad + "\033[0m"
}

// hasRunning 回傳是否有 running 狀態的 session（決定動畫 tick 是否繼續）。
func (m Model) hasRunning() bool {
	for _, item := range m.items {
		if item.Type == ItemSession && item.Session.Status == tmux.StatusRunning {
			return true
		}
	}
	return false
}

// animFrames 是呼吸動畫一個完整週期的 frame 數（40 frames × 50ms = 2 秒）。
const animFrames = 40

// styledStatusIcon 回傳帶動畫效果的狀態圖示。
// running 狀態的 ● 會以 sine 波呼吸效果平滑變化亮度。
func (m Model) styledStatusIcon(status tmux.SessionStatus, icon string) string {
	switch status {
	case tmux.StatusRunning:
		// sine 波：0→1→0 平滑呼吸，亮度 60%～100%
		t := float64(m.animFrame%animFrames) / float64(animFrames)
		brightness := 0.60 + 0.40*((math.Sin(t*2*math.Pi-math.Pi/2)+1)/2)
		r := int(float64(0x91) * brightness)
		g := int(float64(0xE2) * brightness)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x00", r, g)))
		return style.Render(icon)
	case tmux.StatusWaiting:
		return statusWaitingStyle.Render(icon)
	case tmux.StatusError:
		return statusErrorStyle.Render(icon)
	default:
		return statusIdleStyle.Render(icon)
	}
}

// SetItems 設定列表項目（主要用於測試）。
func (m *Model) SetItems(items []ListItem) {
	m.items = items
	if m.cursor >= len(items) && len(items) > 0 {
		m.cursor = len(items) - 1
	}
}

// Cursor 回傳目前游標位置。
func (m Model) Cursor() int {
	return m.cursor
}

// PickerCursor 回傳 picker 模式的游標位置（主要用於測試）。
func (m Model) PickerCursor() int {
	return m.pickerCursor
}

// SetCursor 設定游標位置（主要用於測試）。
func (m *Model) SetCursor(pos int) {
	m.cursor = pos
}
