package store_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wake/tmux-session-menu/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestGroup_CRUD(t *testing.T) {
	s := newTestStore(t)

	// Create
	err := s.CreateGroup("工作專案", 0)
	require.NoError(t, err)
	err = s.CreateGroup("維運", 1)
	require.NoError(t, err)

	// Read
	groups, err := s.ListGroups()
	require.NoError(t, err)
	assert.Len(t, groups, 2)
	assert.Equal(t, "工作專案", groups[0].Name)
	assert.Equal(t, 0, groups[0].SortOrder)
	assert.Equal(t, "維運", groups[1].Name)

	// Update — rename
	err = s.RenameGroup(groups[0].ID, "開發專案")
	require.NoError(t, err)
	groups, err = s.ListGroups()
	require.NoError(t, err)
	assert.Equal(t, "開發專案", groups[0].Name)

	// Delete
	err = s.DeleteGroup(groups[1].ID)
	require.NoError(t, err)
	groups, err = s.ListGroups()
	require.NoError(t, err)
	assert.Len(t, groups, 1)
}

func TestSessionMeta_AssignAndList(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.CreateGroup("dev", 0))
	groups, _ := s.ListGroups()
	groupID := groups[0].ID

	require.NoError(t, s.SetSessionGroup("my-project", groupID, 0))
	require.NoError(t, s.SetSessionGroup("api-server", groupID, 1))
	require.NoError(t, s.SetSessionGroup("standalone", 0, 0))

	metas, err := s.ListSessionMetas(groupID)
	require.NoError(t, err)
	assert.Len(t, metas, 2)
	assert.Equal(t, "my-project", metas[0].SessionName)
	assert.Equal(t, "api-server", metas[1].SessionName)

	ungrouped, err := s.ListSessionMetas(0)
	require.NoError(t, err)
	assert.Len(t, ungrouped, 1)
	assert.Equal(t, "standalone", ungrouped[0].SessionName)
}

func TestSessionMeta_Reorder(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.CreateGroup("dev", 0))
	groups, _ := s.ListGroups()
	gid := groups[0].ID

	require.NoError(t, s.SetSessionGroup("a", gid, 0))
	require.NoError(t, s.SetSessionGroup("b", gid, 1))
	require.NoError(t, s.SetSessionGroup("c", gid, 2))

	// Move c to front
	require.NoError(t, s.SetSessionGroup("c", gid, -1))
	require.NoError(t, s.SetSessionGroup("a", gid, 0))
	require.NoError(t, s.SetSessionGroup("b", gid, 1))

	metas, _ := s.ListSessionMetas(gid)
	assert.Equal(t, "c", metas[0].SessionName)
	assert.Equal(t, "a", metas[1].SessionName)
	assert.Equal(t, "b", metas[2].SessionName)
}

func TestToggleGroupCollapsed(t *testing.T) {
	s := newTestStore(t)

	s.CreateGroup("dev", 0)
	groups, _ := s.ListGroups()
	assert.False(t, groups[0].Collapsed)

	s.ToggleGroupCollapsed(groups[0].ID)
	groups, _ = s.ListGroups()
	assert.True(t, groups[0].Collapsed)

	s.ToggleGroupCollapsed(groups[0].ID)
	groups, _ = s.ListGroups()
	assert.False(t, groups[0].Collapsed)
}

func TestGroup_Reorder(t *testing.T) {
	s := newTestStore(t)

	require.NoError(t, s.CreateGroup("first", 0))
	require.NoError(t, s.CreateGroup("second", 1))
	require.NoError(t, s.CreateGroup("third", 2))

	groups, _ := s.ListGroups()

	require.NoError(t, s.SetGroupOrder(groups[2].ID, -1))
	require.NoError(t, s.SetGroupOrder(groups[0].ID, 0))
	require.NoError(t, s.SetGroupOrder(groups[1].ID, 1))

	groups, _ = s.ListGroups()
	assert.Equal(t, "third", groups[0].Name)
	assert.Equal(t, "first", groups[1].Name)
	assert.Equal(t, "second", groups[2].Name)
}

func TestListAllSessionMetas(t *testing.T) {
	s := newTestStore(t)

	s.CreateGroup("dev", 0)
	groups, _ := s.ListGroups()
	s.SetSessionGroup("alpha", groups[0].ID, 0)
	s.SetSessionGroup("beta", groups[0].ID, 1)

	metas, err := s.ListAllSessionMetas()
	assert.NoError(t, err)
	assert.Len(t, metas, 2)
}
