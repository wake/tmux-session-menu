package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func TestFlattenItems(t *testing.T) {
	groups := []store.Group{
		{ID: 1, Name: "dev", SortOrder: 0},
		{ID: 2, Name: "ops", SortOrder: 1},
	}
	sessions := []tmux.Session{
		{Name: "project-a", GroupName: "dev"},
		{Name: "project-b", GroupName: "dev"},
		{Name: "monitoring", GroupName: "ops"},
		{Name: "standalone"},
	}

	items := ui.FlattenItems(groups, sessions)

	// Groups first
	assert.Equal(t, ui.ItemGroup, items[0].Type)
	assert.Equal(t, "dev", items[0].Group.Name)

	assert.Equal(t, ui.ItemSession, items[1].Type)
	assert.Equal(t, "project-a", items[1].Session.Name)
	assert.Equal(t, ui.ItemSession, items[2].Type)
	assert.Equal(t, "project-b", items[2].Session.Name)

	assert.Equal(t, ui.ItemGroup, items[3].Type)
	assert.Equal(t, "ops", items[3].Group.Name)
	assert.Equal(t, ui.ItemSession, items[4].Type)
	assert.Equal(t, "monitoring", items[4].Session.Name)

	// Then ungrouped sessions
	assert.Equal(t, ui.ItemSession, items[5].Type)
	assert.Equal(t, "standalone", items[5].Session.Name)
}

func TestFlattenItems_CollapsedGroup(t *testing.T) {
	groups := []store.Group{
		{ID: 1, Name: "dev", SortOrder: 0, Collapsed: true},
	}
	sessions := []tmux.Session{
		{Name: "project-a", GroupName: "dev"},
		{Name: "project-b", GroupName: "dev"},
	}

	items := ui.FlattenItems(groups, sessions)
	assert.Len(t, items, 1)
	assert.Equal(t, ui.ItemGroup, items[0].Type)
}

func TestFlattenItems_UngroupedSortOrder(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "charlie", SortOrder: 2},
		{Name: "alpha", SortOrder: 0},
		{Name: "bravo", SortOrder: 1},
	}

	items := ui.FlattenItems(nil, sessions)

	assert.Len(t, items, 3)
	assert.Equal(t, "alpha", items[0].Session.Name)
	assert.Equal(t, "bravo", items[1].Session.Name)
	assert.Equal(t, "charlie", items[2].Session.Name)
}
