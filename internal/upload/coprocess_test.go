package upload

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
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
