package ui

import (
	"sort"

	"github.com/wake/tmux-session-menu/internal/store"
	"github.com/wake/tmux-session-menu/internal/tmux"
)

// ItemType 區分列表項目的種類。
type ItemType int

const (
	ItemSession ItemType = iota
	ItemGroup
	ItemHostTitle // 主機標題列
)

// ListItem 代表列表中的一個項目（session、群組標頭或主機標題）。
type ListItem struct {
	Type      ItemType
	Session   tmux.Session
	Group     store.Group
	HostID    string // 所屬主機 ID
	HostColor string // 主機 accent color
	HostState int    // host title 用：主機連線狀態（0=disabled, 1=connecting, 2=connected, 3=disconnected）
	HostError string // host title 用：錯誤訊息
}

// HostState 常數，對應 hostmgr.HostStatus 的值，避免 ui 直接依賴 hostmgr。
const (
	HostStateDisabled     = 0 // 已停用
	HostStateConnecting   = 1 // 連線中
	HostStateConnected    = 2 // 已連線
	HostStateDisconnected = 3 // 已斷線
)

// HostSnapshotInput 是 FlattenMultiHost 的輸入，避免 ui 直接依賴 hostmgr。
type HostSnapshotInput struct {
	HostID   string
	Name     string
	Color    string
	Status   int // 0=disabled, 1=connecting, 2=connected, 3=disconnected
	Error    string
	Sessions []tmux.Session
	Groups   []store.Group
}

// FlattenItems 將群組與 session 扁平化為一維列表。
// 排列順序：各群組（標頭 + 子 session）→ 未分組 session（按 SortOrder 排序）。
// 已收合的群組不會展開子 session。
func FlattenItems(groups []store.Group, sessions []tmux.Session) []ListItem {
	var items []ListItem

	grouped := make(map[string][]tmux.Session)
	var ungrouped []tmux.Session

	for _, s := range sessions {
		if s.GroupName == "" {
			ungrouped = append(ungrouped, s)
		} else {
			grouped[s.GroupName] = append(grouped[s.GroupName], s)
		}
	}

	// 先放群組（群組內 session 按 SortOrder 排序）
	for _, g := range groups {
		items = append(items, ListItem{Type: ItemGroup, Group: g})
		if !g.Collapsed {
			subs := grouped[g.Name]
			sort.Slice(subs, func(i, j int) bool {
				return subs[i].SortOrder < subs[j].SortOrder
			})
			for _, s := range subs {
				items = append(items, ListItem{Type: ItemSession, Session: s})
			}
		}
	}

	// 未分組按 SortOrder 排序後放在最後
	sort.Slice(ungrouped, func(i, j int) bool {
		return ungrouped[i].SortOrder < ungrouped[j].SortOrder
	})
	for _, s := range ungrouped {
		items = append(items, ListItem{Type: ItemSession, Session: s})
	}

	return items
}

// FlattenMultiHost 將多台主機的快照扁平化為一維列表。
// 每台主機先放一個 ItemHostTitle，若已連線（Status==2）再展開其 sessions/groups。
func FlattenMultiHost(snaps []HostSnapshotInput) []ListItem {
	var items []ListItem

	for _, snap := range snaps {
		// 已停用的主機不顯示
		if snap.Status == HostStateDisabled {
			continue
		}

		// 主機標題列
		items = append(items, ListItem{
			Type:      ItemHostTitle,
			HostID:    snap.HostID,
			HostColor: snap.Color,
			HostState: snap.Status,
			HostError: snap.Error,
		})

		// 僅已連線的主機才展開 sessions
		if snap.Status == HostStateConnected && len(snap.Sessions) > 0 {
			sub := FlattenItems(snap.Groups, snap.Sessions)
			for i := range sub {
				sub[i].HostID = snap.HostID
				sub[i].HostColor = snap.Color
			}
			items = append(items, sub...)
		}
	}

	return items
}
