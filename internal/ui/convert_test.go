package ui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
