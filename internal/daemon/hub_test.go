package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

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
