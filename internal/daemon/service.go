package daemon

import (
	"context"
	"os"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// Service 實作 tsmv1.SessionManagerServer gRPC 介面。
type Service struct {
	tsmv1.UnimplementedSessionManagerServer

	tmuxMgr   *tmux.Manager
	store     *store.Store
	hub       *WatcherHub
	state     *StateManager
	startedAt time.Time
}

// NewService 建立新的 gRPC service。
func NewService(mgr *tmux.Manager, st *store.Store, hub *WatcherHub, state *StateManager) *Service {
	return &Service{
		tmuxMgr:   mgr,
		store:     st,
		hub:       hub,
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

// RenameSession 設定 session 的自訂顯示名稱。
func (s *Service) RenameSession(_ context.Context, req *tsmv1.RenameSessionRequest) (*emptypb.Empty, error) {
	if s.store == nil {
		return &emptypb.Empty{}, nil
	}
	if err := s.store.SetCustomName(req.SessionName, req.CustomName); err != nil {
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
	}, nil
}
