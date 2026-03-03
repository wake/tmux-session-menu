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
)

// ListItem 代表列表中的一個項目（session 或群組標頭）。
type ListItem struct {
	Type    ItemType
	Session tmux.Session
	Group   store.Group
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
