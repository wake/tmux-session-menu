package ui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func TestConvertProtoSessions(t *testing.T) {
	now := time.Now()
	pbSessions := []*tsmv1.Session{
		{
			Name:       "dev",
			Id:         "$1",
			Path:       "/home",
			Attached:   true,
			Activity:   timestamppb.New(now),
			Status:     tsmv1.SessionStatus_SESSION_STATUS_RUNNING,
			AiModel:    "claude-opus-4-6",
			AiSummary:  "重構中",
			AiType:     "claude",
			GroupName:  "work",
			SortOrder:  1,
			CustomName: "開發",
		},
	}

	sessions := ui.ConvertProtoSessions(pbSessions)
	assert.Len(t, sessions, 1)

	s := sessions[0]
	assert.Equal(t, "dev", s.Name)
	assert.Equal(t, "$1", s.ID)
	assert.Equal(t, "/home", s.Path)
	assert.True(t, s.Attached)
	assert.Equal(t, tmux.StatusRunning, s.Status)
	assert.Equal(t, "claude-opus-4-6", s.AIModel)
	assert.Equal(t, "重構中", s.AISummary)
	assert.Equal(t, "claude", s.AiType)
	assert.Equal(t, "work", s.GroupName)
	assert.Equal(t, 1, s.SortOrder)
	assert.Equal(t, "開發", s.CustomName)
}

func TestConvertProtoGroups(t *testing.T) {
	pbGroups := []*tsmv1.Group{
		{Id: 1, Name: "work", SortOrder: 0, Collapsed: false},
		{Id: 2, Name: "personal", SortOrder: 1, Collapsed: true},
	}

	groups := ui.ConvertProtoGroups(pbGroups)
	assert.Len(t, groups, 2)

	assert.Equal(t, int64(1), groups[0].ID)
	assert.Equal(t, "work", groups[0].Name)
	assert.False(t, groups[0].Collapsed)

	assert.Equal(t, int64(2), groups[1].ID)
	assert.Equal(t, "personal", groups[1].Name)
	assert.True(t, groups[1].Collapsed)
}

func TestConvertMultiHostSnapshot(t *testing.T) {
	mhs := &tsmv1.MultiHostSnapshot{
		Hosts: []*tsmv1.HostState{
			{
				HostId: "air",
				Name:   "air",
				Color:  "#5f8787",
				Status: tsmv1.HostStatus_HOST_STATUS_CONNECTED,
				Snapshot: &tsmv1.StateSnapshot{
					Sessions: []*tsmv1.Session{
						{Name: "main", Id: "$0"},
					},
				},
			},
			{
				HostId: "mlab",
				Name:   "mlab",
				Color:  "#73daca",
				Status: tsmv1.HostStatus_HOST_STATUS_CONNECTING,
			},
		},
	}

	inputs := ui.ConvertMultiHostSnapshot(mhs)

	require.Len(t, inputs, 2)
	assert.Equal(t, "air", inputs[0].HostID)
	assert.Equal(t, 2, inputs[0].Status) // HOST_STATUS_CONNECTED
	assert.Len(t, inputs[0].Sessions, 1)
	assert.Equal(t, "mlab", inputs[1].HostID)
	assert.Equal(t, 1, inputs[1].Status) // HOST_STATUS_CONNECTING
	assert.Empty(t, inputs[1].Sessions)
}
