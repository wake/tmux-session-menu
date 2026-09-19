package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wake/tmux-session-menu/internal/config"
	"github.com/wake/tmux-session-menu/internal/hostmgr"
	"github.com/wake/tmux-session-menu/internal/ui"
)

func buildSnaps(mgr *hostmgr.HostManager) []ui.HostSnapshotInput {
	hostSnaps := mgr.Snapshot()
	var inputs []ui.HostSnapshotInput
	for _, hs := range hostSnaps {
		input := ui.HostSnapshotInput{
			HostID: hs.HostID,
			Name:   hs.Name,
			Color:  hs.Color,
			Status: int(hs.Status),
			Error:  hs.Error,
		}
		if hs.Snapshot != nil {
			input.Sessions = ui.ConvertProtoSessions(hs.Snapshot.Sessions)
			input.Groups = ui.ConvertProtoGroups(hs.Snapshot.Groups)
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func main() {
	cfg := config.Default()
	cfg.Hosts = config.EnsureLocal(cfg.Hosts)

	mgr := hostmgr.New()
	for _, h := range cfg.Hosts {
		mgr.AddHost(h)
	}
	for _, h := range mgr.Hosts() {
		if h.IsLocal() {
			h.SetGlobalConfig(cfg)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartAll(ctx)
	defer mgr.Close()

	// 模擬 TUI Init：立即取得快照
	fmt.Println("=== 模擬 TUI Init (immediate) ===")
	snaps := buildSnaps(mgr)
	items := ui.FlattenMultiHost(snaps)
	fmt.Printf("hosts=%d items=%d\n", len(snaps), len(items))
	for _, s := range snaps {
		fmt.Printf("  host=%s status=%d sessions=%d\n", s.HostID, s.Status, len(s.Sessions))
	}

	// 模擬 recvMultiHostCmd：逐個接收通知
	timeout := time.After(5 * time.Second)
	for i := 0; i < 5; i++ {
		fmt.Printf("\n=== 等待通知 #%d ===\n", i+1)
		select {
		case _, ok := <-mgr.SnapshotCh():
			if !ok {
				fmt.Println("channel 關閉")
				return
			}
			snaps = buildSnaps(mgr)
			items = ui.FlattenMultiHost(snaps)
			fmt.Printf("hosts=%d items=%d\n", len(snaps), len(items))
			for _, s := range snaps {
				fmt.Printf("  host=%s status=%d sessions=%d\n", s.HostID, s.Status, len(s.Sessions))
			}
			for _, item := range items {
				switch item.Type {
				case ui.ItemSession:
					fmt.Printf("  → session: %s\n", item.Session.Name)
				case ui.ItemGroup:
					fmt.Printf("  → group: %s\n", item.Group.Name)
				case ui.ItemHostTitle:
					fmt.Printf("  → host-title: %s (state=%d)\n", item.HostID, item.HostState)
				}
			}
			if len(items) > 0 {
				fmt.Printf("\n成功！FlattenMultiHost 回傳 %d 個項目\n", len(items))
				return
			}
		case <-timeout:
			fmt.Println("超時！")
			return
		}
	}
}
