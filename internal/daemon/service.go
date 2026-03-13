package daemon

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/version"
)

// Service 實作 tsmv1.SessionManagerServer gRPC 介面。
type Service struct {
	tsmv1.UnimplementedSessionManagerServer

	tmuxMgr   *tmux.Manager
	store     *store.Store
	hub       *WatcherHub
	mhub      *MultiHostHub // nil = 非 hub 模式
	hubMgr    *HubManager  // nil = 非 hub 模式
	state     *StateManager
	startedAt time.Time
}

// NewService 建立新的 gRPC service。
// mhub 與 hubMgr 為 nil 時表示非 hub 模式。
func NewService(mgr *tmux.Manager, st *store.Store, hub *WatcherHub, mhub *MultiHostHub, hubMgr *HubManager, state *StateManager) *Service {
	return &Service{
		tmuxMgr:   mgr,
		store:     st,
		hub:       hub,
		mhub:      mhub,
		hubMgr:    hubMgr,
		state:     state,
		startedAt: time.Now(),
	}
}

// Watch 建立 server streaming，持續推送 StateSnapshot。
func (s *Service) Watch(_ *tsmv1.WatchRequest, stream tsmv1.SessionManager_WatchServer) error {
	// 推送初始快照
	if snap := s.state.Snapshot(); snap != nil {
		if err := stream.Send(snap); err != nil {
			return err
		}
	}

	ch := s.hub.Register()
	defer s.hub.Unregister(ch)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case snap, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}
}

// CreateSession 建立新的 tmux session。
func (s *Service) CreateSession(_ context.Context, req *tsmv1.CreateSessionRequest) (*emptypb.Empty, error) {
	if err := s.tmuxMgr.NewSession(req.Name, req.Path); err != nil {
		return nil, err
	}
	if req.Command != "" {
		_ = s.tmuxMgr.SendKeys(req.Name, req.Command)
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// KillSession 刪除指定的 tmux session。
func (s *Service) KillSession(_ context.Context, req *tsmv1.KillSessionRequest) (*emptypb.Empty, error) {
	if err := s.tmuxMgr.KillSession(req.Name); err != nil {
		return nil, err
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// RenameSession 設定 session 的自訂顯示名稱，並可選擇重命名 tmux session。
func (s *Service) RenameSession(_ context.Context, req *tsmv1.RenameSessionRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}

	sessionName := req.SessionName

	// 若 NewSessionName 非空且與原名不同，先重命名 tmux session + 遷移 store key
	if newName := req.NewSessionName; newName != "" && newName != sessionName {
		if err := s.tmuxMgr.RenameSession(sessionName, newName); err != nil {
			return nil, err
		}
		if err := s.store.RenameSessionKey(sessionName, newName); err != nil {
			return nil, err
		}
		sessionName = newName
	}

	if err := s.store.SetCustomName(sessionName, req.CustomName); err != nil {
		return nil, err
	}
	// @tsm_name 的同步由 syncUserOptions 在 Scan 時處理（使用 session ID 避免數字 name 歧義）
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// RenameGroup 重命名群組。
func (s *Service) RenameGroup(_ context.Context, req *tsmv1.RenameGroupRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	if err := s.store.RenameGroup(req.GroupId, req.NewName); err != nil {
		return nil, err
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// CreateGroup 建立新的群組。
func (s *Service) CreateGroup(_ context.Context, req *tsmv1.CreateGroupRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	if err := s.store.CreateGroup(req.Name, int(req.SortOrder)); err != nil {
		return nil, err
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// MoveSession 將 session 移動到指定群組。
func (s *Service) MoveSession(_ context.Context, req *tsmv1.MoveSessionRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	if err := s.store.SetSessionGroup(req.SessionName, req.GroupId, int(req.SortOrder)); err != nil {
		return nil, err
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// Reorder 重新排序群組或群組內的 session。
func (s *Service) Reorder(_ context.Context, req *tsmv1.ReorderRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	switch t := req.Target.(type) {
	case *tsmv1.ReorderRequest_Group:
		if err := s.store.SetGroupOrder(t.Group.GroupId, int(t.Group.NewSortOrder)); err != nil {
			return nil, err
		}
	case *tsmv1.ReorderRequest_Session:
		if err := s.store.SetSessionGroup(t.Session.SessionName, t.Session.GroupId, int(t.Session.NewSortOrder)); err != nil {
			return nil, err
		}
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// ToggleCollapse 切換群組的展開/收合狀態。
func (s *Service) ToggleCollapse(_ context.Context, req *tsmv1.ToggleCollapseRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	if err := s.store.ToggleGroupCollapsed(req.GroupId); err != nil {
		return nil, err
	}
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// DaemonStatus 回傳 daemon 的狀態資訊。
func (s *Service) DaemonStatus(_ context.Context, _ *emptypb.Empty) (*tsmv1.DaemonStatusResponse, error) {
	return &tsmv1.DaemonStatusResponse{
		Pid:          int32(os.Getpid()),
		StartedAt:    timestamppb.New(s.startedAt),
		WatcherCount: int32(s.hub.Count()),
		Version:      version.Version,
	}, nil
}

// GetUploadTarget 查詢當前 focused session 的上傳目標。
func (s *Service) GetUploadTarget(_ context.Context, _ *tsmv1.GetUploadTargetRequest) (*tsmv1.GetUploadTargetResponse, error) {
	resp := &tsmv1.GetUploadTargetResponse{}

	// 上傳模式
	if s.state.uploadState.IsUploadMode() {
		resp.UploadMode = true
		resp.SessionName = s.state.uploadState.SessionName()
		if s.tmuxMgr != nil {
			if path, err := s.tmuxMgr.PaneCurrentPath(resp.SessionName); err == nil {
				resp.UploadPath = path
			}
		}
		return resp, nil
	}

	// 自動模式：查詢 focused session
	snap := s.state.Snapshot()
	if snap == nil {
		cfg := loadServerConfig()
		resp.UploadPath = cfg.Upload.RemotePath
		return resp, nil
	}

	// 找 attached session
	for _, sess := range snap.Sessions {
		if sess.Attached {
			resp.SessionName = sess.Name
			// 以 ai_type 判斷 Claude 是否存在（與 status 無關）
			resp.IsClaudeActive = sess.AiType == "claude"
			break
		}
	}

	// IsRemote 由 client 端 coprocess 根據連線來源 overlay，daemon 不需設定。
	cfg := loadServerConfig()
	resp.UploadPath = cfg.Upload.RemotePath

	return resp, nil
}

// SetUploadMode 設定上傳模式開關。
func (s *Service) SetUploadMode(_ context.Context, req *tsmv1.SetUploadModeRequest) (*emptypb.Empty, error) {
	s.state.uploadState.SetMode(req.Enabled, req.SessionName)
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// ReportUploadResult 回報上傳結果。
func (s *Service) ReportUploadResult(_ context.Context, req *tsmv1.ReportUploadResultRequest) (*emptypb.Empty, error) {
	event := &tsmv1.UploadEvent{
		Files: req.Files,
		Error: req.Error,
	}
	s.state.uploadState.AddEvent(event)
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// WatchMultiHost 建立 server streaming，持續推送 MultiHostSnapshot（hub 模式專用）。
func (s *Service) WatchMultiHost(
	req *tsmv1.WatchMultiHostRequest,
	stream tsmv1.SessionManager_WatchMultiHostServer,
) error {
	if s.mhub == nil {
		return status.Error(codes.Unavailable, "not in hub mode")
	}

	// 推送初始快照
	if s.hubMgr != nil {
		if snap := s.hubMgr.Snapshot(); snap != nil && len(snap.Hosts) > 0 {
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}

	ch := s.mhub.Register()
	defer s.mhub.Unregister(ch)

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case snap, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(snap); err != nil {
				return err
			}
		}
	}
}

// ProxyMutation 將操作代理到目標主機的 daemon（hub 模式專用）。
func (s *Service) ProxyMutation(
	ctx context.Context, req *tsmv1.ProxyMutationRequest,
) (*tsmv1.ProxyMutationResponse, error) {
	if s.hubMgr == nil {
		return nil, status.Error(codes.Unavailable, "not in hub mode")
	}
	return s.hubMgr.ProxyMutation(ctx, req)
}

// loadServerConfig 讀取伺服器端的 config.toml，讀取失敗時回傳預設值。
func loadServerConfig() config.Config {
	cfg := config.Default()
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			cfg = loaded
		}
	}
	return cfg
}

// GetConfig 取得 daemon 所在主機的設定。
func (s *Service) GetConfig(_ context.Context, _ *tsmv1.GetConfigRequest) (*tsmv1.GetConfigResponse, error) {
	cfg := loadServerConfig()
	values := []*tsmv1.ConfigValue{
		{Key: "preview_lines", Value: fmt.Sprintf("%d", cfg.PreviewLines)},
		{Key: "poll_interval_sec", Value: fmt.Sprintf("%d", cfg.PollIntervalSec)},
		{Key: "local.bar_bg", Value: cfg.Local.BarBG},
		{Key: "local.bar_fg", Value: cfg.Local.BarFG},
		{Key: "local.badge_bg", Value: cfg.Local.BadgeBG},
		{Key: "local.badge_fg", Value: cfg.Local.BadgeFG},
	}
	return &tsmv1.GetConfigResponse{Values: values}, nil
}

// SetConfig 更新 daemon 所在主機的設定。
func (s *Service) SetConfig(_ context.Context, req *tsmv1.SetConfigRequest) (*emptypb.Empty, error) {
	cfg := loadServerConfig()
	for _, kv := range req.Values {
		switch kv.Key {
		case "preview_lines":
			if v, err := strconv.Atoi(kv.Value); err == nil {
				cfg.PreviewLines = v
			}
		case "poll_interval_sec":
			if v, err := strconv.Atoi(kv.Value); err == nil {
				cfg.PollIntervalSec = v
			}
		case "local.bar_bg":
			cfg.Local.BarBG = kv.Value
		case "local.bar_fg":
			cfg.Local.BarFG = kv.Value
		case "local.badge_bg":
			cfg.Local.BadgeBG = kv.Value
		case "local.badge_fg":
			cfg.Local.BadgeFG = kv.Value
		}
	}
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
