package daemon

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/ai"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// StateManager 負責定期掃描 tmux sessions，偵測差異並透過 WatcherHub 廣播。
type StateManager struct {
	tmuxMgr   *tmux.Manager
	store     *store.Store
	cfg       config.Config
	statusDir string
	hub       *WatcherHub

	mu   sync.RWMutex
	last *tsmv1.StateSnapshot

	scanCh  chan struct{} // debounce channel for Scan() requests
	running int32        // atomic: 1 when Run() is active
}

// NewStateManager 建立新的 StateManager。
func NewStateManager(mgr *tmux.Manager, st *store.Store, cfg config.Config, statusDir string, hub *WatcherHub) *StateManager {
	return &StateManager{
		tmuxMgr:   mgr,
		store:     st,
		cfg:       cfg,
		statusDir: statusDir,
		hub:       hub,
		scanCh:    make(chan struct{}, 1),
	}
}

// Run 啟動掃描迴圈，每秒執行 buildSnapshot 並在有變更時廣播。
// 會阻塞直到 ctx 被取消。
func (sm *StateManager) Run(ctx context.Context) {
	atomic.StoreInt32(&sm.running, 1)
	defer atomic.StoreInt32(&sm.running, 0)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 立即執行一次
	sm.doScan()

	var debounce *time.Timer
	for {
		select {
		case <-ctx.Done():
			if debounce != nil {
				debounce.Stop()
			}
			return
		case <-ticker.C:
			sm.doScan()
		case <-sm.scanCh:
			// 收到 Scan 請求，重設 debounce timer
			if debounce != nil {
				debounce.Stop()
			}
			debounce = time.AfterFunc(50*time.Millisecond, func() {
				select {
				case <-ctx.Done():
				default:
					sm.doScan()
				}
			})
		}
	}
}

// Scan 執行一次掃描並在有變更時廣播（public 封裝，供 service 和測試呼叫）。
// 當 Run() 啟動中，Scan() 透過 debounce channel 合併連續快速呼叫；
// 未啟動時同步執行（測試相容）。
func (sm *StateManager) Scan() {
	if atomic.LoadInt32(&sm.running) == 1 {
		select {
		case sm.scanCh <- struct{}{}:
		default:
		}
		return
	}
	sm.doScan()
}

// doScan 執行一次掃描並在有變更時廣播。
func (sm *StateManager) doScan() {
	snap := sm.BuildSnapshot()
	if snap == nil {
		return
	}
	sm.mu.Lock()
	changed := sm.last == nil || !proto.Equal(sm.last, snap)
	if changed {
		sm.last = snap
	}
	sm.mu.Unlock()
	if changed {
		sm.hub.Broadcast(snap)
	}
}

// Snapshot 回傳最近一次的快照（可能為 nil）。
func (sm *StateManager) Snapshot() *tsmv1.StateSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.last
}

// BuildSnapshot 從 tmux 和 store 建構一個完整的 StateSnapshot。
// 此方法是 public 的，方便測試和 service 層使用。
func (sm *StateManager) BuildSnapshot() *tsmv1.StateSnapshot {
	if sm.tmuxMgr == nil {
		return &tsmv1.StateSnapshot{}
	}

	sessions, err := sm.tmuxMgr.ListSessions()
	if err != nil {
		return sm.Snapshot() // 掃描失敗時保留上次快照
	}

	previewLines := sm.cfg.PreviewLines
	if previewLines <= 0 {
		previewLines = 150
	}

	// 批量取得 pane titles
	paneTitles := make(map[string]string)
	if titles, err := sm.tmuxMgr.ListPaneTitles(); err == nil {
		paneTitles = titles
	}

	// 狀態偵測與 AI 模型偵測
	for i := range sessions {
		var paneContent string
		if content, err := sm.tmuxMgr.CapturePane(sessions[i].Name, previewLines); err == nil {
			paneContent = content
		}
		paneTitle := paneTitles[sessions[i].Name]
		sessions[i].Status = sm.detectStatus(sessions[i].Name, paneTitle, paneContent)
		sessions[i].AIModel = ai.DetectModel(paneContent)
	}

	// 從 store 載入 groups 和 session metas
	var groups []store.Group
	if sm.store != nil {
		groups, _ = sm.store.ListGroups()
		metas, _ := sm.store.ListAllSessionMetas()

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

	return toProtoSnapshot(sessions, groups)
}

// detectStatus 整合三層狀態偵測。
func (sm *StateManager) detectStatus(sessionName, paneTitle, paneContent string) tmux.SessionStatus {
	var input tmux.StatusInput

	if sm.statusDir != "" {
		if hs, err := tmux.ReadHookStatus(sm.statusDir, sessionName); err == nil {
			input.HookStatus = &hs
		}
	}

	if input.HookStatus == nil || !input.HookStatus.IsValid() {
		input.PaneTitle = paneTitle
		input.PaneContent = paneContent
	}

	return tmux.ResolveStatus(input)
}

// toProtoSnapshot 將內部型別轉換為 proto StateSnapshot。
func toProtoSnapshot(sessions []tmux.Session, groups []store.Group) *tsmv1.StateSnapshot {
	snap := &tsmv1.StateSnapshot{
		Sessions: make([]*tsmv1.Session, len(sessions)),
		Groups:   make([]*tsmv1.Group, len(groups)),
	}

	for i, s := range sessions {
		snap.Sessions[i] = &tsmv1.Session{
			Name:       s.Name,
			Id:         s.ID,
			Path:       s.Path,
			Attached:   s.Attached,
			Activity:   timestamppb.New(s.Activity),
			Status:     tsmv1.SessionStatus(s.Status),
			AiModel:    s.AIModel,
			AiSummary:  s.AISummary,
			GroupName:  s.GroupName,
			SortOrder:  int32(s.SortOrder),
			CustomName: s.CustomName,
		}
	}

	for i, g := range groups {
		snap.Groups[i] = &tsmv1.Group{
			Id:        g.ID,
			Name:      g.Name,
			SortOrder: int32(g.SortOrder),
			Collapsed: g.Collapsed,
		}
	}

	return snap
}

// DataDir 回傳設定的資料目錄路徑（已展開）。
func DataDir(cfg config.Config) string {
	return config.ExpandPath(cfg.DataDir)
}

// SocketPath 回傳 unix socket 路徑。
func SocketPath(cfg config.Config) string {
	return filepath.Join(DataDir(cfg), "tsm.sock")
}

// PidPath 回傳 PID 檔案路徑。
func PidPath(cfg config.Config) string {
	return filepath.Join(DataDir(cfg), "tsm.pid")
}
