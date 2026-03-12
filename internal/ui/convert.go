package ui

import (
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// ConvertProtoSessions 將 proto Session 切片轉換為 tmux.Session 切片。
func ConvertProtoSessions(pbSessions []*tsmv1.Session) []tmux.Session {
	sessions := make([]tmux.Session, len(pbSessions))
	for i, pb := range pbSessions {
		var activity time.Time
		if pb.Activity != nil {
			activity = pb.Activity.AsTime()
		}
		sessions[i] = tmux.Session{
			Name:       pb.Name,
			ID:         pb.Id,
			Path:       pb.Path,
			Attached:   pb.Attached,
			Activity:   activity,
			Status:     tmux.SessionStatus(pb.Status),
			AIModel:    pb.AiModel,
			AISummary:  pb.AiSummary,
			GroupName:  pb.GroupName,
			SortOrder:  int(pb.SortOrder),
			CustomName: pb.CustomName,
		}
	}
	return sessions
}

// ConvertMultiHostSnapshot 將 proto MultiHostSnapshot 轉換為 UI 端的 HostSnapshotInput。
func ConvertMultiHostSnapshot(mhs *tsmv1.MultiHostSnapshot) []HostSnapshotInput {
	var inputs []HostSnapshotInput
	for _, hs := range mhs.Hosts {
		input := HostSnapshotInput{
			HostID: hs.HostId,
			Name:   hs.Name,
			Color:  hs.Color,
			Status: int(hs.Status),
			Error:  hs.Error,
		}
		if hs.Snapshot != nil {
			input.Sessions = ConvertProtoSessions(hs.Snapshot.Sessions)
			input.Groups = ConvertProtoGroups(hs.Snapshot.Groups)
		}
		inputs = append(inputs, input)
	}
	return inputs
}

// ConvertProtoGroups 將 proto Group 切片轉換為 store.Group 切片。
func ConvertProtoGroups(pbGroups []*tsmv1.Group) []store.Group {
	groups := make([]store.Group, len(pbGroups))
	for i, pb := range pbGroups {
		groups[i] = store.Group{
			ID:        pb.Id,
			Name:      pb.Name,
			SortOrder: int(pb.SortOrder),
			Collapsed: pb.Collapsed,
		}
	}
	return groups
}
