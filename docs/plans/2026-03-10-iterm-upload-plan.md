# iTerm2 拖曳上傳整合 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 將 iterm-auto-upload 功能整合進 tsm，透過 gRPC daemon 集中偵測邏輯，支援自動上傳模式（遠端 + Claude Code）和手動上傳模式（TUI Shift+U）。

**Architecture:** 新增 `internal/upload/` 包處理上傳邏輯。fileDropCoprocess 由 `tsm iterm-coprocess` 子命令實作，透過 gRPC 查詢 daemon 取得上傳目標資訊。TUI 新增 `ModeUpload` modal 接收上傳結果。Setup 新增 iTerm2 偵測與設定步驟。

**Tech Stack:** Go 1.24+, Protocol Buffers (protoc v6.33.4 + protoc-gen-go v1.36.11), Bubble Tea TUI, gRPC

---

### Task 1: Config — 新增 UploadConfig

**Files:**
- Modify: `internal/config/config.go:42-76`
- Test: `internal/config/config_test.go`

**Step 1: 寫 failing test**

```go
// internal/config/config_test.go — 新增以下測試

func TestDefaultUploadConfig(t *testing.T) {
	cfg := Default()
	if cfg.Upload.RemotePath != "/tmp/iterm-upload" {
		t.Errorf("got %q, want /tmp/iterm-upload", cfg.Upload.RemotePath)
	}
}

func TestLoadUploadConfig(t *testing.T) {
	data := `
[upload]
remote_path = "/data/uploads"
`
	cfg, err := LoadFromString(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upload.RemotePath != "/data/uploads" {
		t.Errorf("got %q, want /data/uploads", cfg.Upload.RemotePath)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/config/... -v -race -run TestDefaultUploadConfig`
Expected: FAIL — `cfg.Upload` 不存在

**Step 3: 實作**

在 `internal/config/config.go` 新增：

```go
// UploadConfig 是拖曳上傳的設定。
type UploadConfig struct {
	RemotePath string `toml:"remote_path"` // 自動模式遠端上傳目錄
}
```

在 `Config` struct 加欄位：

```go
Upload UploadConfig `toml:"upload"` // 拖曳上傳設定
```

在 `Default()` 加預設值：

```go
Upload: UploadConfig{
	RemotePath: "/tmp/iterm-upload",
},
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/config/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): 新增 UploadConfig 結構與預設值"
```

---

### Task 2: Proto — 新增上傳相關 message 與 RPC

**Files:**
- Modify: `api/proto/tsm/v1/tsm.proto:42-45,140-162`
- Regenerate: `api/tsm/v1/tsm.pb.go`, `api/tsm/v1/tsm_grpc.pb.go`

**Step 1: 修改 proto 定義**

在 `tsm.proto` 的 `StateSnapshot` message（45 行後）新增 `UploadEvent`：

```protobuf
// StateSnapshot 是 Watch stream 推送的全量快照。
message StateSnapshot {
  repeated Session sessions = 1;
  repeated Group groups = 2;
  repeated UploadEvent upload_events = 3;
}
```

在 `SetConfigRequest`（137 行）之後新增 message 定義：

```protobuf
// UploadedFile 代表一個已上傳的檔案。
message UploadedFile {
  string local_path = 1;
  string remote_path = 2;
  int64 size_bytes = 3;
}

// UploadEvent 代表一次上傳事件的結果。
message UploadEvent {
  repeated UploadedFile files = 1;
  string error = 2;
  int64 timestamp = 3;
}

// GetUploadTargetRequest 查詢當前 focused session 的上傳目標資訊。
message GetUploadTargetRequest {}

// GetUploadTargetResponse 回傳上傳目標判斷結果。
message GetUploadTargetResponse {
  bool upload_mode = 1;
  bool is_remote = 2;
  bool is_claude_active = 3;
  string ssh_target = 4;
  int32 ssh_port = 5;
  string upload_path = 6;
  string session_name = 7;
}

// SetUploadModeRequest 設定上傳模式開關。
message SetUploadModeRequest {
  bool enabled = 1;
  string session_name = 2;
}

// ReportUploadResultRequest 回報上傳結果。
message ReportUploadResultRequest {
  string session_name = 1;
  repeated UploadedFile files = 2;
  string error = 3;
}
```

在 `SessionManager` service 新增三個 RPC（`SetConfig` 之後）：

```protobuf
// GetUploadTarget 查詢當前 focused session 的上傳目標。
rpc GetUploadTarget(GetUploadTargetRequest) returns (GetUploadTargetResponse);
// SetUploadMode 設定上傳模式開關（TUI 控制）。
rpc SetUploadMode(SetUploadModeRequest) returns (google.protobuf.Empty);
// ReportUploadResult 回報上傳結果（coprocess 呼叫）。
rpc ReportUploadResult(ReportUploadResultRequest) returns (google.protobuf.Empty);
```

**Step 2: 生成 Go 碼**

Run:
```bash
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  -I api/proto \
  api/proto/tsm/v1/tsm.proto
```

將生成的 `tsm.pb.go` 和 `tsm_grpc.pb.go` 移到 `api/tsm/v1/`（如果 protoc 輸出位置不同的話）。

**Step 3: 確認編譯通過**

Run: `go build ./...`
Expected: 成功，無錯誤

**Step 4: Commit**

```bash
git add api/proto/tsm/v1/tsm.proto api/tsm/v1/tsm.pb.go api/tsm/v1/tsm_grpc.pb.go
git commit -m "feat(proto): 新增上傳相關 message 與 RPC 定義"
```

---

### Task 3: Daemon — UploadState 狀態追蹤

**Files:**
- Create: `internal/daemon/upload_state.go`
- Test: `internal/daemon/upload_state_test.go`

**Step 1: 寫 failing test**

```go
// internal/daemon/upload_state_test.go
package daemon

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

func TestUploadState_SetMode(t *testing.T) {
	us := NewUploadState()

	// 預設關閉
	if us.IsUploadMode() {
		t.Fatal("expected upload mode off by default")
	}

	us.SetMode(true, "my-session")
	if !us.IsUploadMode() {
		t.Fatal("expected upload mode on")
	}
	if us.SessionName() != "my-session" {
		t.Errorf("got %q, want my-session", us.SessionName())
	}

	us.SetMode(false, "")
	if us.IsUploadMode() {
		t.Fatal("expected upload mode off")
	}
}

func TestUploadState_AddEvent(t *testing.T) {
	us := NewUploadState()

	us.AddEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 1024},
		},
	})

	events := us.DrainEvents()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Files[0].LocalPath != "/tmp/a.png" {
		t.Errorf("got %q", events[0].Files[0].LocalPath)
	}

	// drain 後應為空
	if len(us.DrainEvents()) != 0 {
		t.Fatal("expected empty after drain")
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/daemon/... -v -race -run TestUploadState`
Expected: FAIL — `NewUploadState` 不存在

**Step 3: 實作**

```go
// internal/daemon/upload_state.go
package daemon

import (
	"sync"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// UploadState 追蹤上傳模式狀態和上傳事件。
type UploadState struct {
	mu          sync.RWMutex
	enabled     bool
	sessionName string
	events      []*tsmv1.UploadEvent
}

// NewUploadState 建立新的 UploadState。
func NewUploadState() *UploadState {
	return &UploadState{}
}

// SetMode 設定上傳模式。
func (us *UploadState) SetMode(enabled bool, sessionName string) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.enabled = enabled
	us.sessionName = sessionName
	if !enabled {
		us.events = nil // 關閉時清空事件
	}
}

// IsUploadMode 回傳是否處於上傳模式。
func (us *UploadState) IsUploadMode() bool {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.enabled
}

// SessionName 回傳上傳模式的目標 session。
func (us *UploadState) SessionName() string {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.sessionName
}

// AddEvent 新增一個上傳事件。
func (us *UploadState) AddEvent(event *tsmv1.UploadEvent) {
	us.mu.Lock()
	defer us.mu.Unlock()
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}
	us.events = append(us.events, event)
}

// DrainEvents 取出所有事件並清空。
func (us *UploadState) DrainEvents() []*tsmv1.UploadEvent {
	us.mu.Lock()
	defer us.mu.Unlock()
	events := us.events
	us.events = nil
	return events
}
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/daemon/... -v -race -run TestUploadState`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/daemon/upload_state.go internal/daemon/upload_state_test.go
git commit -m "feat(daemon): 新增 UploadState 狀態追蹤"
```

---

### Task 4: Daemon — 整合 UploadState 到 StateManager

**Files:**
- Modify: `internal/daemon/state.go:21-34,102-117`
- Modify: `internal/daemon/daemon.go`（NewStateManager 呼叫處）

**Step 1: 寫 failing test**

```go
// internal/daemon/upload_state_test.go — 追加

func TestStateManager_UploadEventsInSnapshot(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	sm.uploadState = NewUploadState()

	// 加入上傳事件
	sm.uploadState.AddEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/test.png", RemotePath: "/data/test.png"},
		},
	})

	// BuildSnapshot 應包含 upload_events
	snap := sm.BuildSnapshot()
	if len(snap.UploadEvents) != 1 {
		t.Fatalf("got %d upload events, want 1", len(snap.UploadEvents))
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/daemon/... -v -race -run TestStateManager_UploadEvents`
Expected: FAIL — `sm.uploadState` 不存在

**Step 3: 實作**

在 `StateManager` struct 加欄位：

```go
uploadState *UploadState
```

在 `NewStateManager` 初始化：

```go
uploadState: NewUploadState(),
```

在 `BuildSnapshot()` 末尾（return 前），附加 upload events：

```go
snap := toProtoSnapshot(sessions, groups)
snap.UploadEvents = sm.uploadState.DrainEvents()
return snap
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/daemon/... -v -race -run TestStateManager_UploadEvents`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/daemon/state.go internal/daemon/upload_state_test.go
git commit -m "feat(daemon): 整合 UploadState 到 StateManager snapshot"
```

---

### Task 5: Daemon Service — GetUploadTarget RPC

**Files:**
- Modify: `internal/daemon/service.go`
- Test: `internal/daemon/service_test.go`

**Step 1: 寫 failing test**

```go
// internal/daemon/service_test.go — 新增或追加

func TestService_GetUploadTarget_LocalSession(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, sm)

	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	// 本機 session，非遠端
	if resp.IsRemote {
		t.Error("expected local session")
	}
	if resp.UploadMode {
		t.Error("expected upload mode off")
	}
}

func TestService_GetUploadTarget_UploadMode(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, sm)

	sm.uploadState.SetMode(true, "my-session")

	resp, err := svc.GetUploadTarget(context.Background(), &tsmv1.GetUploadTargetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.UploadMode {
		t.Error("expected upload mode on")
	}
	if resp.SessionName != "my-session" {
		t.Errorf("got session %q, want my-session", resp.SessionName)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/daemon/... -v -race -run TestService_GetUploadTarget`
Expected: FAIL — `GetUploadTarget` 方法不存在

**Step 3: 實作**

在 `internal/daemon/service.go` 新增：

```go
// GetUploadTarget 查詢當前 focused session 的上傳目標。
func (s *Service) GetUploadTarget(_ context.Context, _ *tsmv1.GetUploadTargetRequest) (*tsmv1.GetUploadTargetResponse, error) {
	resp := &tsmv1.GetUploadTargetResponse{}

	// 上傳模式
	if s.state.uploadState.IsUploadMode() {
		resp.UploadMode = true
		resp.SessionName = s.state.uploadState.SessionName()

		// 取得 session 工作目錄
		if s.tmuxMgr != nil {
			if path, err := s.tmuxMgr.PaneCurrentPath(resp.SessionName); err == nil {
				resp.UploadPath = path
			}
		}
		return resp, nil
	}

	// 自動模式：查詢 focused session
	if s.tmuxMgr == nil {
		return resp, nil
	}

	snap := s.state.Snapshot()
	if snap == nil {
		return resp, nil
	}

	// 找到 attached session
	var focused *tsmv1.Session
	for _, sess := range snap.Sessions {
		if sess.Attached {
			focused = sess
			break
		}
	}
	if focused == nil {
		return resp, nil
	}

	resp.SessionName = focused.Name
	resp.IsClaudeActive = focused.Status == tsmv1.SessionStatus_SESSION_STATUS_RUNNING ||
		focused.Status == tsmv1.SessionStatus_SESSION_STATUS_WAITING

	// TODO: 遠端 session 偵測需要 hostmgr 整合，暫時回傳 is_remote=false
	// 後續 Task 會補完遠端判斷邏輯

	cfg := loadServerConfig()
	resp.UploadPath = cfg.Upload.RemotePath

	return resp, nil
}
```

注意：`tmux.Manager.PaneCurrentPath()` 如果尚未存在，需要在 Task 6 補。這裡先不依賴它，上傳模式的 `upload_path` 可先留空。

**Step 4: 跑測試確認通過**

Run: `go test ./internal/daemon/... -v -race -run TestService_GetUploadTarget`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat(daemon): 實作 GetUploadTarget RPC"
```

---

### Task 6: Daemon Service — SetUploadMode 與 ReportUploadResult RPC

**Files:**
- Modify: `internal/daemon/service.go`
- Test: `internal/daemon/service_test.go`

**Step 1: 寫 failing test**

```go
func TestService_SetUploadMode(t *testing.T) {
	hub := NewWatcherHub()
	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, sm)

	_, err := svc.SetUploadMode(context.Background(), &tsmv1.SetUploadModeRequest{
		Enabled:     true,
		SessionName: "test-sess",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !sm.uploadState.IsUploadMode() {
		t.Error("expected upload mode on")
	}
}

func TestService_ReportUploadResult(t *testing.T) {
	hub := NewWatcherHub()
	ch := hub.Register()

	sm := NewStateManager(nil, nil, config.Default(), "", hub)
	svc := NewService(nil, nil, hub, sm)

	_, err := svc.ReportUploadResult(context.Background(), &tsmv1.ReportUploadResultRequest{
		SessionName: "test-sess",
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 512},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// 事件應被加入 uploadState，Scan 後透過 snapshot 廣播
	events := sm.uploadState.DrainEvents()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	_ = ch // hub channel 用於驗證廣播（Scan 會觸發）
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/daemon/... -v -race -run "TestService_SetUploadMode|TestService_ReportUploadResult"`
Expected: FAIL

**Step 3: 實作**

```go
// SetUploadMode 設定上傳模式開關。
func (s *Service) SetUploadMode(_ context.Context, req *tsmv1.SetUploadModeRequest) (*emptypb.Empty, error) {
	s.state.uploadState.SetMode(req.Enabled, req.SessionName)
	s.state.Scan()
	return &emptypb.Empty{}, nil
}

// ReportUploadResult 回報上傳結果。
func (s *Service) ReportUploadResult(_ context.Context, req *tsmv1.ReportUploadResultRequest) (*emptypb.Empty, error) {
	event := &tsmv1.UploadEvent{
		Files: req.Files,
		Error: req.Error,
	}
	s.state.uploadState.AddEvent(event)
	s.state.Scan()
	return &emptypb.Empty{}, nil
}
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/daemon/... -v -race -run "TestService_SetUploadMode|TestService_ReportUploadResult"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/daemon/service.go internal/daemon/service_test.go
git commit -m "feat(daemon): 實作 SetUploadMode 與 ReportUploadResult RPCs"
```

---

### Task 7: Client — 新增上傳相關方法

**Files:**
- Modify: `internal/client/client.go`
- Test: 整合測試（透過 daemon 測試覆蓋，此處只確認編譯）

**Step 1: 實作 client 方法**

在 `internal/client/client.go` 末尾（`SetConfig` 之後）新增：

```go
// GetUploadTarget 查詢當前 focused session 的上傳目標。
func (c *Client) GetUploadTarget(ctx context.Context) (*tsmv1.GetUploadTargetResponse, error) {
	return c.rpc.GetUploadTarget(ctx, &tsmv1.GetUploadTargetRequest{})
}

// SetUploadMode 設定上傳模式開關。
func (c *Client) SetUploadMode(ctx context.Context, enabled bool, sessionName string) error {
	_, err := c.rpc.SetUploadMode(ctx, &tsmv1.SetUploadModeRequest{
		Enabled:     enabled,
		SessionName: sessionName,
	})
	return err
}

// ReportUploadResult 回報上傳結果。
func (c *Client) ReportUploadResult(ctx context.Context, sessionName string, files []*tsmv1.UploadedFile, uploadErr string) error {
	_, err := c.rpc.ReportUploadResult(ctx, &tsmv1.ReportUploadResultRequest{
		SessionName: sessionName,
		Files:       files,
		Error:       uploadErr,
	})
	return err
}
```

**Step 2: 確認編譯**

Run: `go build ./...`
Expected: 成功

**Step 3: Commit**

```bash
git add internal/client/client.go
git commit -m "feat(client): 新增上傳相關 gRPC 方法"
```

---

### Task 8: Upload 包 — 核心上傳邏輯

**Files:**
- Create: `internal/upload/upload.go`
- Create: `internal/upload/upload_test.go`

**Step 1: 寫 failing test**

```go
// internal/upload/upload_test.go
package upload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyLocal(t *testing.T) {
	// 準備來源檔案
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// 準備目標目錄
	dstDir := t.TempDir()

	results, err := CopyLocal([]string{srcFile}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].RemotePath != filepath.Join(dstDir, "test.txt") {
		t.Errorf("got remote_path %q", results[0].RemotePath)
	}
	if results[0].SizeBytes != 5 {
		t.Errorf("got size %d, want 5", results[0].SizeBytes)
	}

	// 驗證檔案內容
	data, err := os.ReadFile(filepath.Join(dstDir, "test.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("got %q, want hello", string(data))
	}
}

func TestFormatBracketedPaste(t *testing.T) {
	paths := []string{"/data/a.png", "/data/b.pdf"}
	got := FormatBracketedPaste(paths)
	want := "\x1b[200~/data/a.png /data/b.pdf\x1b[201~"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatLocalPaths(t *testing.T) {
	paths := []string{"/Users/wake/a.png", "/Users/wake/my file.pdf"}
	got := FormatLocalPaths(paths)
	want := "/Users/wake/a.png /Users/wake/my\\ file.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/upload/... -v -race`
Expected: FAIL — 套件不存在

**Step 3: 實作**

```go
// internal/upload/upload.go
package upload

import (
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// CopyLocal 複製檔案到本機目標目錄。
func CopyLocal(files []string, dstDir string) ([]*tsmv1.UploadedFile, error) {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dstDir, err)
	}

	var results []*tsmv1.UploadedFile
	for _, src := range files {
		dst := filepath.Join(dstDir, filepath.Base(src))
		size, err := copyFile(src, dst)
		if err != nil {
			return results, fmt.Errorf("copy %s: %w", src, err)
		}
		results = append(results, &tsmv1.UploadedFile{
			LocalPath:  src,
			RemotePath: dst,
			SizeBytes:  size,
		})
	}
	return results, nil
}

// ScpUpload 透過 scp 上傳檔案到遠端。
func ScpUpload(sshTarget string, sshPort int, dstDir string, files []string) ([]*tsmv1.UploadedFile, error) {
	// 建立遠端目錄
	mkdirArgs := []string{sshTarget, "mkdir", "-p", dstDir}
	if sshPort != 0 && sshPort != 22 {
		mkdirArgs = append([]string{"-p", fmt.Sprintf("%d", sshPort)}, mkdirArgs...)
	}
	if out, err := osexec.Command("ssh", mkdirArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ssh mkdir: %s: %w", string(out), err)
	}

	// scp 上傳
	scpArgs := []string{}
	if sshPort != 0 && sshPort != 22 {
		scpArgs = append(scpArgs, "-P", fmt.Sprintf("%d", sshPort))
	}
	scpArgs = append(scpArgs, files...)
	scpArgs = append(scpArgs, fmt.Sprintf("%s:%s/", sshTarget, dstDir))

	if out, err := osexec.Command("scp", scpArgs...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("scp: %s: %w", string(out), err)
	}

	var results []*tsmv1.UploadedFile
	for _, src := range files {
		info, _ := os.Stat(src)
		var size int64
		if info != nil {
			size = info.Size()
		}
		results = append(results, &tsmv1.UploadedFile{
			LocalPath:  src,
			RemotePath: filepath.Join(dstDir, filepath.Base(src)),
			SizeBytes:  size,
		})
	}
	return results, nil
}

// FormatBracketedPaste 格式化遠端路徑為 bracketed paste 序列。
func FormatBracketedPaste(paths []string) string {
	return "\x1b[200~" + strings.Join(paths, " ") + "\x1b[201~"
}

// FormatLocalPaths 格式化本地路徑，空格需轉義。
func FormatLocalPaths(paths []string) string {
	escaped := make([]string, len(paths))
	for i, p := range paths {
		escaped[i] = strings.ReplaceAll(p, " ", "\\ ")
	}
	return strings.Join(escaped, " ")
}

// copyFile 複製單一檔案，回傳檔案大小。
func copyFile(src, dst string) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	n, err := io.Copy(out, in)
	return n, err
}
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/upload/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/upload/upload.go internal/upload/upload_test.go
git commit -m "feat(upload): 新增核心上傳邏輯（CopyLocal, ScpUpload, 格式化）"
```

---

### Task 9: Upload 包 — Coprocess 決策邏輯

**Files:**
- Create: `internal/upload/coprocess.go`
- Create: `internal/upload/coprocess_test.go`

**Step 1: 寫 failing test**

```go
// internal/upload/coprocess_test.go
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
		IsRemote:      true,
		IsClaudeActive: true,
		SshTarget:     "user@remote",
		UploadPath:    "/tmp/iterm-upload",
		SessionName:   "dev",
	}
	action := DecideAction(resp)
	if action != ActionAutoUpload {
		t.Errorf("got %v, want ActionAutoUpload", action)
	}
}

func TestDecideAction_Passthrough(t *testing.T) {
	// 本機 session
	resp := &tsmv1.GetUploadTargetResponse{
		IsRemote: false,
	}
	action := DecideAction(resp)
	if action != ActionPassthrough {
		t.Errorf("got %v, want ActionPassthrough", action)
	}

	// 遠端但 Claude 未運行
	resp2 := &tsmv1.GetUploadTargetResponse{
		IsRemote:       true,
		IsClaudeActive: false,
	}
	action2 := DecideAction(resp2)
	if action2 != ActionPassthrough {
		t.Errorf("got %v, want ActionPassthrough", action2)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/upload/... -v -race -run TestDecideAction`
Expected: FAIL — `DecideAction` 不存在

**Step 3: 實作**

```go
// internal/upload/coprocess.go
package upload

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
	"github.com/wake/tmux-session-menu/internal/client"
	"github.com/wake/tmux-session-menu/internal/config"
)

// Action 代表 coprocess 的決策結果。
type Action int

const (
	ActionPassthrough Action = iota // 輸出本地路徑
	ActionAutoUpload               // 自動上傳 + bracketed paste
	ActionUploadMode               // 上傳模式 + 回報 TUI
)

// DecideAction 根據 daemon 回傳的上傳目標判斷動作。
func DecideAction(resp *tsmv1.GetUploadTargetResponse) Action {
	if resp.UploadMode {
		return ActionUploadMode
	}
	if resp.IsRemote && resp.IsClaudeActive {
		return ActionAutoUpload
	}
	return ActionPassthrough
}

// RunCoprocess 是 tsm iterm-coprocess 子命令的主邏輯。
// filenames 是 iTerm2 傳入的本地檔案路徑。
func RunCoprocess(filenames []string) {
	logger := setupLogger()

	cfg := config.Default()
	cfgPath := config.ExpandPath("~/.config/tsm/config.toml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if loaded, err := config.LoadFromString(string(data)); err == nil {
			cfg = loaded
		}
	}

	// 連線 daemon
	c, err := client.Dial(cfg)
	if err != nil {
		logger.Printf("daemon 連線失敗: %v", err)
		fmt.Print(FormatLocalPaths(filenames))
		return
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 查詢上傳目標
	resp, err := c.GetUploadTarget(ctx)
	if err != nil {
		logger.Printf("GetUploadTarget 失敗: %v", err)
		fmt.Print(FormatLocalPaths(filenames))
		return
	}

	action := DecideAction(resp)
	logger.Printf("action=%d session=%s remote=%v claude=%v upload_mode=%v",
		action, resp.SessionName, resp.IsRemote, resp.IsClaudeActive, resp.UploadMode)

	switch action {
	case ActionUploadMode:
		handleUploadMode(ctx, c, resp, filenames, logger)

	case ActionAutoUpload:
		handleAutoUpload(ctx, c, resp, filenames, logger)

	default:
		fmt.Print(FormatLocalPaths(filenames))
	}
}

func handleUploadMode(ctx context.Context, c *client.Client, resp *tsmv1.GetUploadTargetResponse, files []string, logger *log.Logger) {
	var results []*tsmv1.UploadedFile
	var uploadErr string

	if resp.IsRemote && resp.SshTarget != "" {
		r, err := ScpUpload(resp.SshTarget, int(resp.SshPort), resp.UploadPath, files)
		if err != nil {
			uploadErr = err.Error()
		}
		results = r
	} else {
		r, err := CopyLocal(files, resp.UploadPath)
		if err != nil {
			uploadErr = err.Error()
		}
		results = r
	}

	// 回報結果（不輸出到 stdout）
	if err := c.ReportUploadResult(ctx, resp.SessionName, results, uploadErr); err != nil {
		logger.Printf("ReportUploadResult 失敗: %v", err)
	}
}

func handleAutoUpload(ctx context.Context, c *client.Client, resp *tsmv1.GetUploadTargetResponse, files []string, logger *log.Logger) {
	results, err := ScpUpload(resp.SshTarget, int(resp.SshPort), resp.UploadPath, files)
	if err != nil {
		// 失敗 → fallback 本地路徑 + 通知 daemon
		logger.Printf("自動上傳失敗: %v", err)
		_ = c.ReportUploadResult(ctx, resp.SessionName, nil, err.Error())
		fmt.Print(FormatLocalPaths(files))
		return
	}

	// 成功 → bracketed paste 輸出遠端路徑
	remotePaths := make([]string, len(results))
	for i, r := range results {
		remotePaths[i] = r.RemotePath
	}
	fmt.Print(FormatBracketedPaste(remotePaths))
}

// ParseFilenames 解析 iTerm2 傳入的 filenames 字串（shell-quoted 空格分隔）。
func ParseFilenames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	// iTerm2 用空格分隔，路徑含空格時會 shell-quote
	// 簡化處理：以空格分隔，但合併被引號包裹的部分
	var files []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case !inQuote && (ch == '\'' || ch == '"'):
			inQuote = true
			quoteChar = ch
		case inQuote && ch == quoteChar:
			inQuote = false
		case !inQuote && ch == '\\' && i+1 < len(raw):
			i++
			current.WriteByte(raw[i])
		case !inQuote && ch == ' ':
			if current.Len() > 0 {
				files = append(files, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		files = append(files, current.String())
	}
	return files
}

func setupLogger() *log.Logger {
	logDir := config.ExpandPath("~/.config/tsm/logs")
	_ = os.MkdirAll(logDir, 0755)
	f, err := os.OpenFile(
		logDir+"/upload.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return log.New(os.Stderr, "[tsm-upload] ", log.LstdFlags)
	}
	return log.New(f, "", log.LstdFlags)
}
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/upload/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/upload/coprocess.go internal/upload/coprocess_test.go
git commit -m "feat(upload): 實作 coprocess 決策邏輯與 ParseFilenames"
```

---

### Task 10: ParseFilenames 測試補強

**Files:**
- Modify: `internal/upload/coprocess_test.go`

**Step 1: 寫測試**

```go
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
```

**Step 2: 跑測試確認通過**

Run: `go test ./internal/upload/... -v -race -run TestParseFilenames`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/upload/coprocess_test.go
git commit -m "test(upload): 補強 ParseFilenames 測試案例"
```

---

### Task 11: TUI — ModeUpload 與 Upload Modal

**Files:**
- Modify: `internal/ui/mode.go`
- Create: `internal/ui/upload_modal.go`
- Create: `internal/ui/upload_modal_test.go`
- Modify: `internal/ui/app.go`（Shift+U 鍵綁定、UploadResultMsg 處理）

**Step 1: 寫 failing test**

```go
// internal/ui/upload_modal_test.go
package ui

import (
	"testing"

	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

func TestUploadModal_InitialState(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects/myapp")
	if m.sessionName != "my-session" {
		t.Errorf("got session %q", m.sessionName)
	}
	if len(m.results) != 0 {
		t.Error("expected empty results")
	}
}

func TestUploadModal_AddResult(t *testing.T) {
	m := newUploadModal("my-session", "air-2019", "~/projects/myapp")

	m.addEvent(&tsmv1.UploadEvent{
		Files: []*tsmv1.UploadedFile{
			{LocalPath: "/tmp/a.png", RemotePath: "/data/a.png", SizeBytes: 1024},
		},
	})

	if len(m.results) != 1 {
		t.Fatalf("got %d results, want 1", len(m.results))
	}

	// 加入失敗事件
	m.addEvent(&tsmv1.UploadEvent{
		Error: "scp timeout",
	})
	if len(m.results) != 2 {
		t.Fatalf("got %d results, want 2", len(m.results))
	}
	if m.results[1].err != "scp timeout" {
		t.Errorf("got err %q", m.results[1].err)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run TestUploadModal`
Expected: FAIL

**Step 3: 實作 mode.go 修改**

在 `internal/ui/mode.go` 新增：

```go
ModeUpload // 上傳模式
```

加在 `ModeNewSession` 之後。

**Step 4: 實作 upload_modal.go**

```go
// internal/ui/upload_modal.go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tsmv1 "github.com/wake/tmux-session-menu/api/tsm/v1"
)

// uploadResult 代表一次上傳事件的顯示資料。
type uploadResult struct {
	files []*tsmv1.UploadedFile
	err   string
}

// uploadModal 是上傳模式的 TUI 元件。
type uploadModal struct {
	sessionName string
	hostLabel   string
	targetPath  string
	results     []uploadResult
}

// newUploadModal 建立上傳 modal。
func newUploadModal(sessionName, hostLabel, targetPath string) uploadModal {
	return uploadModal{
		sessionName: sessionName,
		hostLabel:   hostLabel,
		targetPath:  targetPath,
	}
}

// addEvent 加入一個上傳事件。
func (m *uploadModal) addEvent(event *tsmv1.UploadEvent) {
	m.results = append(m.results, uploadResult{
		files: event.Files,
		err:   event.Error,
	})
}

// view 渲染上傳 modal 畫面。
func (m uploadModal) view(width, height int) string {
	var b strings.Builder

	title := headerStyle.Render("上傳模式")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  目標: %s %s\n", m.hostLabel, m.targetPath))
	b.WriteString(fmt.Sprintf("  (session: %s)\n", m.sessionName))
	b.WriteString("\n")

	if len(m.results) == 0 {
		b.WriteString(dimStyle.Render("  拖曳檔案到 iTerm2 視窗即可上傳"))
		b.WriteString("\n")
	} else {
		for _, r := range m.results {
			if r.err != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e")).Render(
					fmt.Sprintf("  ✗ %s", r.err)))
				b.WriteString("\n")
				continue
			}
			for _, f := range r.files {
				sizeStr := formatSize(f.SizeBytes)
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a")).Render(
					fmt.Sprintf("  ✓ %-30s %s", f.RemotePath, sizeStr)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  [Esc] 關閉"))

	return b.String()
}

// formatSize 格式化檔案大小。
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
```

**Step 5: 跑測試確認通過**

Run: `go test ./internal/ui/... -v -race -run TestUploadModal`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/ui/mode.go internal/ui/upload_modal.go internal/ui/upload_modal_test.go
git commit -m "feat(ui): 新增 ModeUpload 與 Upload Modal 元件"
```

---

### Task 12: TUI — 整合 Upload Modal 到 app.go

**Files:**
- Modify: `internal/ui/app.go`（Model struct、Update、View）

**Step 1: 寫 failing test**

```go
// internal/ui/upload_modal_test.go — 追加

func TestModel_ShiftU_EntersUploadMode(t *testing.T) {
	m := NewModel(Deps{})
	m.mode = ModeNormal

	// 模擬 Shift+U 按鍵
	// 無 client 時不應進入上傳模式
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}}
	newModel, _ := m.Update(msg)
	updated := newModel.(Model)
	if updated.mode == ModeUpload {
		t.Error("should not enter upload mode without client")
	}
}
```

注意：完整的 Shift+U 整合需要 mock client，這裡先測試無 client 的保護邏輯。

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/ui/... -v -race -run TestModel_ShiftU`
Expected: FAIL — `ModeUpload` 相關處理不存在

**Step 3: 實作**

在 `Model` struct 新增欄位：

```go
uploadModal uploadModal // 上傳 modal
```

在 `Update()` 的 `ModeNormal` 分支中，處理 Shift+U：

```go
case "U": // Shift+U：上傳模式
	if m.deps.Client == nil && m.deps.HostMgr == nil {
		break
	}
	// 取得當前 focused session
	sessionName := m.currentSessionName()
	if sessionName == "" {
		break
	}
	m.mode = ModeUpload
	m.uploadModal = newUploadModal(sessionName, m.currentHostLabel(), m.currentSessionPath())
	return m, m.setUploadModeCmd(true, sessionName)
```

在 `Update()` 新增 `ModeUpload` 處理：

```go
case ModeUpload:
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc {
			sessionName := m.uploadModal.sessionName
			m.mode = ModeNormal
			return m, m.setUploadModeCmd(false, sessionName)
		}
	}
```

處理 `SnapshotMsg` 中的 `UploadEvents`（在現有的 snapshot 處理邏輯中）：

```go
if m.mode == ModeUpload && len(snap.UploadEvents) > 0 {
	for _, event := range snap.UploadEvents {
		m.uploadModal.addEvent(event)
	}
}
```

在 `View()` 新增 `ModeUpload` 渲染：

```go
case ModeUpload:
	return m.uploadModal.view(m.width, m.height)
```

輔助方法：

```go
func (m Model) setUploadModeCmd(enabled bool, sessionName string) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Client != nil {
			_ = m.deps.Client.SetUploadMode(context.Background(), enabled, sessionName)
		}
		return nil
	}
}
```

`currentSessionName()`, `currentHostLabel()`, `currentSessionPath()` 根據現有的 items/cursor 邏輯實作。

**Step 4: 跑測試確認通過**

Run: `go test ./internal/ui/... -v -race`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/upload_modal.go internal/ui/upload_modal_test.go
git commit -m "feat(ui): 整合 Upload Modal 到主 TUI（Shift+U 觸發）"
```

---

### Task 13: CLI — iterm-coprocess 子命令

**Files:**
- Modify: `cmd/tsm/main.go`

**Step 1: 實作子命令路由**

在 `main()` 的 switch 中，`"setup"` 之前新增：

```go
case "iterm-coprocess":
	runItermCoprocess(args[1:])
	return
```

實作函數：

```go
// runItermCoprocess 執行 iTerm2 fileDropCoprocess 邏輯。
func runItermCoprocess(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: tsm iterm-coprocess <filenames>")
		os.Exit(1)
	}
	// args 可能是多個檔案路徑，也可能是一個包含所有路徑的字串
	var filenames []string
	for _, arg := range args {
		parsed := upload.ParseFilenames(arg)
		filenames = append(filenames, parsed...)
	}
	if len(filenames) == 0 {
		return
	}
	upload.RunCoprocess(filenames)
}
```

**Step 2: 確認編譯**

Run: `go build ./cmd/tsm/`
Expected: 成功

**Step 3: Commit**

```bash
git add cmd/tsm/main.go
git commit -m "feat(cli): 新增 iterm-coprocess 子命令"
```

---

### Task 14: Setup — iTerm2 偵測與安裝

**Files:**
- Create: `internal/setup/iterm.go`
- Create: `internal/setup/iterm_test.go`
- Modify: `cmd/tsm/main.go`（runSetup 中新增 component）

**Step 1: 寫 failing test**

```go
// internal/setup/iterm_test.go
package setup

import (
	"testing"
)

func TestItermCoprocessCommand(t *testing.T) {
	cmd := ItermCoprocessCommand("/usr/local/bin/tsm")
	want := `/usr/local/bin/tsm iterm-coprocess \(filenames)`
	if cmd != want {
		t.Errorf("got %q, want %q", cmd, want)
	}
}
```

**Step 2: 跑測試確認失敗**

Run: `go test ./internal/setup/... -v -race -run TestItermCoprocess`
Expected: FAIL

**Step 3: 實作**

```go
// internal/setup/iterm.go
package setup

import (
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
)

// DetectIterm2 檢查是否安裝了 iTerm2。
func DetectIterm2() bool {
	// 檢查 app bundle
	if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
		return true
	}
	// 檢查 defaults
	out, err := osexec.Command("defaults", "read", "com.googlecode.iterm2", "Default Bookmark Guid").CombinedOutput()
	return err == nil && len(out) > 0
}

// ItermCoprocessCommand 回傳 fileDropCoprocess 的設定值。
func ItermCoprocessCommand(tsmPath string) string {
	return fmt.Sprintf(`%s iterm-coprocess \(filenames)`, tsmPath)
}

// InstallItermCoprocess 設定 iTerm2 的 fileDropCoprocess。
func InstallItermCoprocess() (string, error) {
	tsmPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("取得 tsm 路徑失敗: %w", err)
	}

	cmd := ItermCoprocessCommand(tsmPath)
	if err := osexec.Command("defaults", "write", "com.googlecode.iterm2",
		"fileDropCoprocess", "-string", cmd).Run(); err != nil {
		return "", fmt.Errorf("defaults write 失敗: %w", err)
	}

	return fmt.Sprintf("已設定 fileDropCoprocess\n  手動設定: iTerm2 Settings → Advanced → 搜尋 \"file drop\"\n  值: %s\n  需重啟 iTerm2 生效", cmd), nil
}

// UninstallItermCoprocess 移除 iTerm2 的 fileDropCoprocess 設定。
func UninstallItermCoprocess() (string, error) {
	if err := osexec.Command("defaults", "delete", "com.googlecode.iterm2",
		"fileDropCoprocess").Run(); err != nil {
		// 如果本來就沒設定，不算錯誤
		if strings.Contains(err.Error(), "does not exist") {
			return "fileDropCoprocess 未設定，跳過", nil
		}
		return "", fmt.Errorf("defaults delete 失敗: %w", err)
	}
	return "已移除 fileDropCoprocess 設定", nil
}

// DetectItermCoprocessInstalled 檢查 fileDropCoprocess 是否已設定為 tsm。
func DetectItermCoprocessInstalled() bool {
	out, err := osexec.Command("defaults", "read", "com.googlecode.iterm2",
		"fileDropCoprocess").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "tsm iterm-coprocess")
}
```

**Step 4: 跑測試確認通過**

Run: `go test ./internal/setup/... -v -race -run TestItermCoprocess`
Expected: PASS

**Step 5: 整合到 runSetup**

在 `cmd/tsm/main.go` 的 `runSetup()` 中，在現有 components 清單裡新增：

```go
// 偵測 iTerm2，有的話加入 coprocess 選項
if setup.DetectIterm2() {
	components = append(components, setup.Component{
		Label:     "iTerm2 拖曳上傳",
		InstallFn: setup.InstallItermCoprocess,
		UninstallFn: setup.UninstallItermCoprocess,
		Checked:   true,
		Installed: setup.DetectItermCoprocessInstalled(),
		Note:      "設定 fileDropCoprocess，支援拖曳檔案自動上傳到遠端",
	})
}
```

**Step 6: 確認編譯**

Run: `go build ./cmd/tsm/`
Expected: 成功

**Step 7: Commit**

```bash
git add internal/setup/iterm.go internal/setup/iterm_test.go cmd/tsm/main.go
git commit -m "feat(setup): 新增 iTerm2 拖曳上傳偵測與安裝"
```

---

### Task 15: tmux.Manager — PaneCurrentPath 方法

**Files:**
- Modify: `internal/tmux/session.go`
- Test: `internal/tmux/session_test.go`

**Step 1: 寫 failing test**

```go
// 測試 PaneCurrentPath 呼叫 tmux display-message 取得路徑
func TestManager_PaneCurrentPath_Command(t *testing.T) {
	// 此方法需要 tmux 環境，標記為整合測試
	if os.Getenv("TMUX") == "" {
		t.Skip("需要 tmux 環境")
	}
	mgr := NewManager()
	path, err := mgr.PaneCurrentPath("")
	if err != nil {
		t.Fatal(err)
	}
	// 至少應回傳非空路徑
	if path == "" {
		t.Error("expected non-empty path")
	}
}
```

**Step 2: 實作**

在 `internal/tmux/session.go` 新增：

```go
// PaneCurrentPath 取得指定 session 的當前 pane 工作目錄。
// sessionName 為空時取得當前 session。
func (m *Manager) PaneCurrentPath(sessionName string) (string, error) {
	args := []string{"display-message", "-p", "#{pane_current_path}"}
	if sessionName != "" {
		args = []string{"display-message", "-t", sessionName, "-p", "#{pane_current_path}"}
	}
	out, err := m.run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
```

**Step 3: 確認編譯**

Run: `go build ./...`
Expected: 成功

**Step 4: Commit**

```bash
git add internal/tmux/session.go internal/tmux/session_test.go
git commit -m "feat(tmux): 新增 PaneCurrentPath 方法"
```

---

### Task 16: 全量測試與版本推進

**Step 1: 跑全部測試**

Run: `make test`
Expected: 全部 PASS

**Step 2: 跑 lint**

Run: `make lint`
Expected: 無警告

**Step 3: 更新版本號**

讀取 `VERSION` 檔案，推進 MINOR 版本（新功能）。
例如 `0.25.2` → `0.26.0`。

```bash
echo "0.26.0" > VERSION
```

**Step 4: 確認建置**

Run: `make build`
Expected: 成功，`bin/tsm` 產生

**Step 5: Commit**

```bash
git add VERSION
git commit -m "chore: bump version to 0.26.0"
```

---

### Task 17: 整合測試 — 端到端驗證

**Step 1: 手動驗證 coprocess 基本流程**

```bash
# 測試 passthrough（daemon 未啟動時）
echo "test" | bin/tsm iterm-coprocess /tmp/test.txt
# 預期輸出: /tmp/test.txt

# 啟動 daemon
bin/tsm daemon start

# 測試 GetUploadTarget
# （透過 TUI 驗證 Shift+U 進入上傳模式）
bin/tsm --inline
# 在 TUI 中按 Shift+U，確認進入上傳模式畫面
# 按 Esc 退出
```

**Step 2: 驗證 iTerm2 設定**

```bash
# 測試偵測
bin/tsm setup
# 確認 iTerm2 選項出現

# 測試手動設定指令
defaults read com.googlecode.iterm2 fileDropCoprocess
```

**Step 3: Commit（如有修正）**

```bash
git add -A
git commit -m "fix: 整合測試修正"
```
