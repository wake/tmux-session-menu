package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFlattenMultiHost(t *testing.T) {
	// 2 台主機：local（已連線、1 session）+ remote（已連線、2 sessions 含 1 群組）
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "local",
			Name:     "Local",
			Color:    "#00ff00",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}},
			Groups:   nil,
		},
		{
			HostID: "remote",
			Name:   "Remote",
			Color:  "#ff0000",
			Status: 2, // connected
			Sessions: []tmux.Session{
				{Name: "web", GroupName: "work", SortOrder: 0},
				{Name: "api", GroupName: "", SortOrder: 1},
			},
			Groups: []store.Group{
				{ID: 1, Name: "work", SortOrder: 0, Collapsed: false},
			},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	// 預期順序：HostTitle(local), Session(dev), HostTitle(remote), Group(work), Session(web), Session(api)
	if assert.Len(t, items, 6) {
		assert.Equal(t, ui.ItemHostTitle, items[0].Type)
		assert.Equal(t, "local", items[0].HostID)

		assert.Equal(t, ui.ItemSession, items[1].Type)
		assert.Equal(t, "local", items[1].HostID)
		assert.Equal(t, "dev", items[1].Session.Name)

		assert.Equal(t, ui.ItemHostTitle, items[2].Type)
		assert.Equal(t, "remote", items[2].HostID)

		assert.Equal(t, ui.ItemGroup, items[3].Type)
		assert.Equal(t, "remote", items[3].HostID)
		assert.Equal(t, "work", items[3].Group.Name)

		assert.Equal(t, ui.ItemSession, items[4].Type)
		assert.Equal(t, "remote", items[4].HostID)
		assert.Equal(t, "web", items[4].Session.Name)

		assert.Equal(t, ui.ItemSession, items[5].Type)
		assert.Equal(t, "remote", items[5].HostID)
		assert.Equal(t, "api", items[5].Session.Name)
	}

	// 驗證 HostColor 標記
	assert.Equal(t, "#00ff00", items[0].HostColor)
	assert.Equal(t, "#00ff00", items[1].HostColor)
	assert.Equal(t, "#ff0000", items[3].HostColor)
}

func TestFlattenMultiHostDisconnected(t *testing.T) {
	// 1 台主機：連線中，無 session
	snaps := []ui.HostSnapshotInput{
		{
			HostID: "remote-a",
			Name:   "Remote A",
			Color:  "#0000ff",
			Status: 1, // connecting
		},
	}

	items := ui.FlattenMultiHost(snaps)

	if assert.Len(t, items, 1) {
		assert.Equal(t, ui.ItemHostTitle, items[0].Type)
		assert.Equal(t, "remote-a", items[0].HostID)
		assert.Equal(t, 1, items[0].HostState)
	}
}

func TestFlattenMultiHostMixed(t *testing.T) {
	// 3 台主機：connected、disconnected、connected
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "h1",
			Name:     "Host1",
			Color:    "#aaa",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "s1", SortOrder: 0}},
		},
		{
			HostID: "h2",
			Name:   "Host2",
			Color:  "#bbb",
			Status: 3, // disconnected
			Error:  "timeout",
		},
		{
			HostID:   "h3",
			Name:     "Host3",
			Color:    "#ccc",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "s2", SortOrder: 0}, {Name: "s3", SortOrder: 1}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	// 預期：HostTitle(h1), Session(s1), HostTitle(h2), HostTitle(h3), Session(s2), Session(s3)
	if assert.Len(t, items, 6) {
		assert.Equal(t, ui.ItemHostTitle, items[0].Type)
		assert.Equal(t, "h1", items[0].HostID)

		assert.Equal(t, ui.ItemSession, items[1].Type)
		assert.Equal(t, "h1", items[1].HostID)

		assert.Equal(t, ui.ItemHostTitle, items[2].Type)
		assert.Equal(t, "h2", items[2].HostID)
		assert.Equal(t, "timeout", items[2].HostError)
		assert.Equal(t, 3, items[2].HostState)

		assert.Equal(t, ui.ItemHostTitle, items[3].Type)
		assert.Equal(t, "h3", items[3].HostID)

		assert.Equal(t, ui.ItemSession, items[4].Type)
		assert.Equal(t, "h3", items[4].HostID)

		assert.Equal(t, ui.ItemSession, items[5].Type)
		assert.Equal(t, "h3", items[5].HostID)
	}
}

func TestFlattenMultiHostDisabled(t *testing.T) {
	// 已停用的主機不應出現在列表中
	snaps := []ui.HostSnapshotInput{
		{
			HostID: "local",
			Name:   "Local",
			Color:  "#aaa",
			Status: 0, // disabled
		},
		{
			HostID:   "mlab",
			Name:     "mlab",
			Color:    "#00ff00",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	// disabled 的 local 不應出現，僅有 mlab 的 HostTitle + Session
	if assert.Len(t, items, 2) {
		assert.Equal(t, ui.ItemHostTitle, items[0].Type)
		assert.Equal(t, "mlab", items[0].HostID)

		assert.Equal(t, ui.ItemSession, items[1].Type)
		assert.Equal(t, "dev", items[1].Session.Name)
	}
}

func TestFlattenItemsUnchanged(t *testing.T) {
	// 驗證原有 FlattenItems 仍正常運作（不帶 HostID）
	groups := []store.Group{
		{ID: 1, Name: "grp", SortOrder: 0, Collapsed: false},
	}
	sessions := []tmux.Session{
		{Name: "a", GroupName: "grp", SortOrder: 1},
		{Name: "b", GroupName: "", SortOrder: 0},
	}

	items := ui.FlattenItems(groups, sessions)

	if assert.Len(t, items, 3) {
		assert.Equal(t, ui.ItemGroup, items[0].Type)
		assert.Equal(t, "grp", items[0].Group.Name)

		assert.Equal(t, ui.ItemSession, items[1].Type)
		assert.Equal(t, "a", items[1].Session.Name)

		assert.Equal(t, ui.ItemSession, items[2].Type)
		assert.Equal(t, "b", items[2].Session.Name)
	}

	// 確認無 HostID（向後相容）
	for i, item := range items {
		assert.Empty(t, item.HostID, "items[%d].HostID should be empty", i)
	}
}

func TestFlattenMultiHostLocalOnlyHidesTitle(t *testing.T) {
	// 只有 local enabled → 不顯示 host title
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "local",
			Name:     "local",
			Color:    "#5f8787",
			Status:   2, // connected
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}, {Name: "web", SortOrder: 1}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	// 應只有 2 個 session item，無 host title
	require.Len(t, items, 2)
	assert.Equal(t, ui.ItemSession, items[0].Type)
	assert.Equal(t, "dev", items[0].Session.Name)
	assert.Equal(t, ui.ItemSession, items[1].Type)
	assert.Equal(t, "web", items[1].Session.Name)
}

func TestFlattenMultiHostLocalOnlyWithDisabledOthers(t *testing.T) {
	// local enabled + 其他 disabled → 只有 local，隱藏標題
	snaps := []ui.HostSnapshotInput{
		{
			HostID:   "local",
			Name:     "local",
			Color:    "#5f8787",
			Status:   2,
			Sessions: []tmux.Session{{Name: "dev", SortOrder: 0}},
		},
		{
			HostID: "remote-a",
			Name:   "remote-a",
			Color:  "#ff0000",
			Status: 0, // disabled
		},
	}

	items := ui.FlattenMultiHost(snaps)

	require.Len(t, items, 1)
	assert.Equal(t, ui.ItemSession, items[0].Type)
	assert.Equal(t, "dev", items[0].Session.Name)
}

func TestFlattenMultiHostSingleRemoteShowsTitle(t *testing.T) {
	// 只有一台 remote enabled（local disabled）→ 顯示 title
	snaps := []ui.HostSnapshotInput{
		{
			HostID: "local",
			Name:   "local",
			Status: 0, // disabled
		},
		{
			HostID:   "remote-a",
			Name:     "remote-a",
			Color:    "#ff0000",
			Status:   2,
			Sessions: []tmux.Session{{Name: "web", SortOrder: 0}},
		},
	}

	items := ui.FlattenMultiHost(snaps)

	require.Len(t, items, 2)
	assert.Equal(t, ui.ItemHostTitle, items[0].Type)
	assert.Equal(t, "remote-a", items[0].HostID)
	assert.Equal(t, ui.ItemSession, items[1].Type)
}
