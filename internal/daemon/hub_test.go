package daemon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

// mockMutationClient 實作 MutationClient 介面供測試使用。
type mockMutationClient struct {
	killFn   func(ctx context.Context, name string) error
	renameFn func(ctx context.Context, sessionName, customName, newSessionName string) error
	createFn func(ctx context.Context, name, path, command string) error
	moveFn   func(ctx context.Context, sessionName string, groupID int64, sortOrder int) error
}

func (m *mockMutationClient) KillSession(ctx context.Context, name string) error {
	if m.killFn != nil {
		return m.killFn(ctx, name)
	}
	return nil
}

func (m *mockMutationClient) RenameSession(ctx context.Context, sessionName, customName, newSessionName string) error {
	if m.renameFn != nil {
		return m.renameFn(ctx, sessionName, customName, newSessionName)
	}
	return nil
}

func (m *mockMutationClient) CreateSession(ctx context.Context, name, path, command string) error {
	if m.createFn != nil {
		return m.createFn(ctx, name, path, command)
	}
	return nil
}

func (m *mockMutationClient) MoveSession(ctx context.Context, sessionName string, groupID int64, sortOrder int) error {
	if m.moveFn != nil {
		return m.moveFn(ctx, sessionName, groupID, sortOrder)
	}
	return nil
}

func TestHubManager_BuildMultiHostSnapshot(t *testing.T) {
	mgr := NewHubManager(NewMultiHostHub())

	localSnap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "main", Id: "$0"}},
	}
	remoteSnap := &tsmv1.StateSnapshot{
		Sessions: []*tsmv1.Session{{Name: "dev", Id: "$1"}},
	}

	mgr.updateHostSnapshot("local", config.HostEntry{
		Name: "air", Color: "#5f8787",
	}, tsmv1.HostStatus_HOST_STATUS_CONNECTED, localSnap, "")

	mgr.updateHostSnapshot("mlab", config.HostEntry{
		Name: "mlab", Color: "#73daca",
	}, tsmv1.HostStatus_HOST_STATUS_CONNECTED, remoteSnap, "")

	snap := mgr.Snapshot()

	require.Len(t, snap.Hosts, 2)
	assert.Equal(t, "local", snap.Hosts[0].HostId)
	assert.Equal(t, "mlab", snap.Hosts[1].HostId)
	assert.Len(t, snap.Hosts[0].Snapshot.Sessions, 1)
	assert.Len(t, snap.Hosts[1].Snapshot.Sessions, 1)
}

func TestHubManager_AddHost(t *testing.T) {
	mgr := NewHubManager(NewMultiHostHub())

	mgr.AddHost(config.HostEntry{Name: "air", Enabled: true})
	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	snap := mgr.Snapshot()
	require.Len(t, snap.Hosts, 2)
	assert.Equal(t, "air", snap.Hosts[0].HostId)
	assert.Equal(t, "mlab", snap.Hosts[1].HostId)
	assert.Equal(t, tsmv1.HostStatus_HOST_STATUS_DISABLED, snap.Hosts[0].Status)
}

func TestHubManager_UpdateBroadcasts(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	ch := mhub.Register()

	mgr.updateHostSnapshot("local", config.HostEntry{Name: "air"},
		tsmv1.HostStatus_HOST_STATUS_CONNECTED,
		&tsmv1.StateSnapshot{Sessions: []*tsmv1.Session{{Name: "main"}}},
		"")

	select {
	case got := <-ch:
		require.Len(t, got.Hosts, 1)
		assert.Equal(t, "local", got.Hosts[0].HostId)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestHubManager_Snapshot_Empty(t *testing.T) {
	mgr := NewHubManager(NewMultiHostHub())
	snap := mgr.Snapshot()
	assert.NotNil(t, snap)
	assert.Empty(t, snap.Hosts)
}

func TestHubManager_ProxyMutation_RoutesToCorrectHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	var killedOn string
	mockClient := &mockMutationClient{
		killFn: func(ctx context.Context, name string) error {
			killedOn = name
			return nil
		},
	}
	mgr.setHostClient("mlab", mockClient)

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "mlab",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "dev",
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "dev", killedOn)
}

func TestHubManager_ProxyMutation_LocalHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	mgr.AddHost(config.HostEntry{Name: "air", Enabled: true})

	var killedLocal string
	mgr.localMutationFn = func(req *tsmv1.ProxyMutationRequest) error {
		killedLocal = req.SessionName
		return nil
	}

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "air",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "main",
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "main", killedLocal)
}

func TestHubManager_ProxyMutation_UnknownHost(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "nonexistent",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "x",
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "unknown host")
}

func TestHubManager_ProxyMutation_NotConnected(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})
	// 未設定 client — 模擬主機尚未連線

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "mlab",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "dev",
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "not connected")
}

func TestHubManager_ProxyMutation_RemoteError(t *testing.T) {
	mhub := NewMultiHostHub()
	mgr := NewHubManager(mhub)

	mgr.AddHost(config.HostEntry{Name: "mlab", Address: "mlab", Enabled: true})

	mockClient := &mockMutationClient{
		killFn: func(ctx context.Context, name string) error {
			return fmt.Errorf("tmux: session not found: %s", name)
		},
	}
	mgr.setHostClient("mlab", mockClient)

	resp, err := mgr.ProxyMutation(context.Background(), &tsmv1.ProxyMutationRequest{
		HostId:      "mlab",
		Type:        tsmv1.MutationType_MUTATION_KILL_SESSION,
		SessionName: "gone",
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "session not found")
}
