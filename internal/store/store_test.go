package store_test

import (
	"path/filepath"
	"testing"
	"time"

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

func TestSetCustomName(t *testing.T) {
	s := newTestStore(t)

	// 設定自訂名稱
	err := s.SetCustomName("my-session", "我的開發環境")
	require.NoError(t, err)

	// 透過 ListAllSessionMetas 驗證
	metas, err := s.ListAllSessionMetas()
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "my-session", metas[0].SessionName)
	assert.Equal(t, "我的開發環境", metas[0].CustomName)
}

func TestRenameSessionKey(t *testing.T) {
	s := newTestStore(t)

	// 設定初始資料
	require.NoError(t, s.CreateGroup("dev", 0))
	groups, _ := s.ListGroups()
	require.NoError(t, s.SetSessionGroup("old-name", groups[0].ID, 0))
	require.NoError(t, s.SetCustomName("old-name", "我的專案"))

	// 遷移 key
	err := s.RenameSessionKey("old-name", "new-name")
	require.NoError(t, err)

	// 驗證舊 key 不存在、新 key 保留所有欄位
	metas, err := s.ListAllSessionMetas()
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "new-name", metas[0].SessionName)
	assert.Equal(t, "我的專案", metas[0].CustomName)
	assert.Equal(t, groups[0].ID, metas[0].GroupID)
}

func TestGetCustomName_NotFound(t *testing.T) {
	s := newTestStore(t)

	// 查詢不存在的 session，應回傳空字串且無錯誤
	name, err := s.GetCustomName("non-existent")
	assert.NoError(t, err)
	assert.Equal(t, "", name)
}

func TestGetCustomName_Found(t *testing.T) {
	s := newTestStore(t)

	// 先設定自訂名稱
	require.NoError(t, s.SetCustomName("my-session", "我的開發環境"))

	// 查詢應回傳正確的自訂名稱
	name, err := s.GetCustomName("my-session")
	require.NoError(t, err)
	assert.Equal(t, "我的開發環境", name)
}

func TestSetCustomName_Update(t *testing.T) {
	s := newTestStore(t)

	// 第一次設定
	err := s.SetCustomName("my-session", "第一版名稱")
	require.NoError(t, err)

	// 第二次設定（應覆蓋）
	err = s.SetCustomName("my-session", "第二版名稱")
	require.NoError(t, err)

	// 驗證只有一筆且值為第二次的
	metas, err := s.ListAllSessionMetas()
	require.NoError(t, err)
	require.Len(t, metas, 1)
	assert.Equal(t, "第二版名稱", metas[0].CustomName)
}

func TestPathHistory_AddAndList(t *testing.T) {
	s := newTestStore(t)

	assert.NoError(t, s.AddPathHistory("/home/user/project-a"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-b"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-c"))
	assert.NoError(t, s.AddPathHistory("/home/user/project-d"))

	paths, err := s.RecentPaths(3)
	assert.NoError(t, err)
	assert.Len(t, paths, 3)
	assert.Equal(t, "/home/user/project-d", paths[0])
	assert.Equal(t, "/home/user/project-c", paths[1])
	assert.Equal(t, "/home/user/project-b", paths[2])
}

func TestPathHistory_DuplicateUpdatesTimestamp(t *testing.T) {
	s := newTestStore(t)

	assert.NoError(t, s.AddPathHistory("/a"))
	time.Sleep(10 * time.Millisecond) // 確保時間戳不同
	assert.NoError(t, s.AddPathHistory("/b"))
	time.Sleep(10 * time.Millisecond)
	assert.NoError(t, s.AddPathHistory("/a")) // 重複

	paths, err := s.RecentPaths(3)
	assert.NoError(t, err)
	assert.Equal(t, "/a", paths[0]) // /a 最近使用
	assert.Equal(t, "/b", paths[1])
}
