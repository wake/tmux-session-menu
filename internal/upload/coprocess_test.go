package upload

import (
	"context"
	"errors"
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/config"
)

func TestDecideAction_UploadMode(t *testing.T) {
	resp := &tsmv1.GetUploadTargetResponse{
		UploadMode:  true,
		SessionName: "my-session",
		UploadPath:  "/home/user/project",
		IsRemote:    true,
		SshTarget:   "user@remote",
	}
	action := DecideAction(resp)
	if action != ActionUploadMode {
		t.Errorf("got %v, want ActionUploadMode", action)
	}
}

func TestDecideAction_AutoUpload(t *testing.T) {
	resp := &tsmv1.GetUploadTargetResponse{
		IsRemote:       true,
		IsClaudeActive: true,
		SshTarget:      "user@remote",
		UploadPath:     "/tmp/iterm-upload",
		SessionName:    "dev",
	}
	action := DecideAction(resp)
	if action != ActionAutoUpload {
		t.Errorf("got %v, want ActionAutoUpload", action)
	}
}

func TestDecideAction_Passthrough_Local(t *testing.T) {
	resp := &tsmv1.GetUploadTargetResponse{
		IsRemote: false,
	}
	if DecideAction(resp) != ActionPassthrough {
		t.Error("expected ActionPassthrough for local session")
	}
}

func TestDecideAction_Passthrough_RemoteNoClaude(t *testing.T) {
	resp := &tsmv1.GetUploadTargetResponse{
		IsRemote:       true,
		IsClaudeActive: false,
	}
	if DecideAction(resp) != ActionPassthrough {
		t.Error("expected ActionPassthrough when Claude not active")
	}
}

func TestDecideAction_UploadModeTakesPrecedence(t *testing.T) {
	resp := &tsmv1.GetUploadTargetResponse{
		UploadMode:     true,
		IsRemote:       true,
		IsClaudeActive: true,
	}
	if DecideAction(resp) != ActionUploadMode {
		t.Error("expected upload mode to take precedence over auto-upload")
	}
}

// --- resolveRemoteTarget 測試 ---

func TestResolveRemoteTarget_FindsRemoteDaemon(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Enabled: true},
		{Name: "mlab", Address: "mlab", Enabled: true},
	}

	remoteResp := &tsmv1.GetUploadTargetResponse{
		SessionName:    "dev",
		IsClaudeActive: true,
		UploadPath:     "/tmp/iterm-upload",
	}

	sockExists := func(string) bool { return true }
	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		return remoteResp, nil
	}

	resp := resolveRemoteTarget(context.Background(), hosts, sockExists, dialGet)
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if !resp.IsRemote {
		t.Error("expected IsRemote=true")
	}
	if resp.SshTarget != "mlab" {
		t.Errorf("got SshTarget=%q, want mlab", resp.SshTarget)
	}
	if resp.SessionName != "dev" {
		t.Errorf("got SessionName=%q, want dev", resp.SessionName)
	}
	if !resp.IsClaudeActive {
		t.Error("expected IsClaudeActive=true")
	}
}

func TestResolveRemoteTarget_SkipsLocalHost(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "local", Address: "", Enabled: true},
	}
	called := false
	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		called = true
		return nil, errors.New("should not be called")
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return true }, dialGet)
	if resp != nil {
		t.Error("expected nil for local-only hosts")
	}
	if called {
		t.Error("should not dial for local host")
	}
}

func TestResolveRemoteTarget_SkipsDisabledHost(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: false},
	}
	called := false
	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		called = true
		return nil, errors.New("should not be called")
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return true }, dialGet)
	if resp != nil {
		t.Error("expected nil for disabled host")
	}
	if called {
		t.Error("should not dial for disabled host")
	}
}

func TestResolveRemoteTarget_NoTunnelSocket(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}
	called := false
	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		called = true
		return nil, errors.New("should not be called")
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return false }, dialGet)
	if resp != nil {
		t.Error("expected nil when no tunnel socket")
	}
	if called {
		t.Error("should not dial when socket missing")
	}
}

func TestResolveRemoteTarget_SkipsEmptySession(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}

	// Remote daemon 回傳空 session（沒有 attached session 也沒有 upload mode）
	emptyResp := &tsmv1.GetUploadTargetResponse{
		UploadPath: "/tmp/iterm-upload",
	}

	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		return emptyResp, nil
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return true }, dialGet)
	if resp != nil {
		t.Error("expected nil when remote has no session")
	}
}

func TestResolveRemoteTarget_DialError_SkipsToNext(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "host-a", Address: "host-a", Enabled: true},
		{Name: "host-b", Address: "host-b", Enabled: true},
	}

	remoteResp := &tsmv1.GetUploadTargetResponse{
		SessionName:    "work",
		IsClaudeActive: true,
		UploadPath:     "/tmp/iterm-upload",
	}

	callCount := 0
	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		callCount++
		if callCount == 1 {
			return nil, errors.New("connection refused")
		}
		return remoteResp, nil
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return true }, dialGet)
	if resp == nil {
		t.Fatal("expected non-nil response from second host")
	}
	if resp.SshTarget != "host-b" {
		t.Errorf("got SshTarget=%q, want host-b", resp.SshTarget)
	}
	if callCount != 2 {
		t.Errorf("expected 2 dial attempts, got %d", callCount)
	}
}

func TestResolveRemoteTarget_UploadModeReturned(t *testing.T) {
	hosts := []config.HostEntry{
		{Name: "mlab", Address: "mlab", Enabled: true},
	}

	// Remote daemon 回傳 upload mode（即使 session 空也要回傳）
	remoteResp := &tsmv1.GetUploadTargetResponse{
		UploadMode:  true,
		SessionName: "coding",
		UploadPath:  "/home/user/project",
	}

	dialGet := func(_ context.Context, _ string) (*tsmv1.GetUploadTargetResponse, error) {
		return remoteResp, nil
	}

	resp := resolveRemoteTarget(context.Background(), hosts, func(string) bool { return true }, dialGet)
	if resp == nil {
		t.Fatal("expected non-nil response for upload mode")
	}
	if !resp.UploadMode {
		t.Error("expected UploadMode=true")
	}
	if !resp.IsRemote {
		t.Error("expected IsRemote=true")
	}
	if resp.SshTarget != "mlab" {
		t.Errorf("got SshTarget=%q, want mlab", resp.SshTarget)
	}
}

func TestReconstructFilenames(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no spaces",
			args: []string{"/tmp/a.png", "/tmp/b.png"},
			want: []string{"/tmp/a.png", "/tmp/b.png"},
		},
		{
			name: "single file with spaces (CleanShot)",
			args: []string{"/Users/wake/Desktop/CleanShot", "2026-03-11", "at", "12.08.54@2x.png"},
			want: []string{"/Users/wake/Desktop/CleanShot 2026-03-11 at 12.08.54@2x.png"},
		},
		{
			name: "mixed",
			args: []string{"/tmp/a.png", "/Users/wake/Desktop/Clean", "Shot.png"},
			want: []string{"/tmp/a.png", "/Users/wake/Desktop/Clean Shot.png"},
		},
		{
			name: "two files both with spaces",
			args: []string{"/tmp/a", "b.png", "/tmp/c", "d.png"},
			want: []string{"/tmp/a b.png", "/tmp/c d.png"},
		},
		{
			name: "three files mixed",
			args: []string{"/tmp/ok.png", "/tmp/has", "space.png", "/tmp/also", "has", "space.jpg"},
			want: []string{"/tmp/ok.png", "/tmp/has space.png", "/tmp/also has space.jpg"},
		},
		{
			name: "directory with spaces",
			args: []string{"/Users/wake/My", "Documents/"},
			want: []string{"/Users/wake/My Documents/"},
		},
		{
			name: "single file",
			args: []string{"/tmp/a.png"},
			want: []string{"/tmp/a.png"},
		},
		{
			name: "empty",
			args: nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReconstructFilenames(tt.args)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseFilenames(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"single", "/tmp/a.png", []string{"/tmp/a.png"}},
		{"multiple", "/tmp/a.png /tmp/b.pdf", []string{"/tmp/a.png", "/tmp/b.pdf"}},
		{"quoted spaces", `"/tmp/my file.png" /tmp/b.pdf`, []string{"/tmp/my file.png", "/tmp/b.pdf"}},
		{"escaped spaces", `/tmp/my\ file.png /tmp/b.pdf`, []string{"/tmp/my file.png", "/tmp/b.pdf"}},
		{"empty", "", nil},
		{"single quoted", `'/tmp/my file.png'`, []string{"/tmp/my file.png"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFilenames(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d]=%q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
