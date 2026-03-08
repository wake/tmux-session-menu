# Multi-Host Mix 設計文件

## 概覽

tsm 支援同時連接多個 tmux daemon（本機 + 多台遠端），在單一 TUI 畫面中混合顯示所有主機的 session。

### 初期目標（by-host 模式）

1. 一次連接多個 daemon，每組 session 列表上方顯示帶背景色的 host title
2. `--remote hostA --remote hostB` 多遠端參數
3. `h` 快捷鍵開啟主機管理面板
4. 面板支援新增 / 刪除 / 啟用停用 / 上下排序

### 遠期目標

- 兩種檢視模式：by-host（初期）/ custom（自由分組、跨主機搬運排序）

---

## §1 HostManager 核心結構

新增 `internal/hostmgr/` 套件，作為 UI 與多個 Client 之間的中間層。

### HostManager

```go
type HostManager struct {
    mu         sync.RWMutex
    hosts      []*Host
    snapshotCh chan struct{}  // 任一 host 快照更新時通知
}

func (m *HostManager) Add(hostCfg HostConfig) error
func (m *HostManager) Remove(hostID string) error
func (m *HostManager) Enable(hostID string) error
func (m *HostManager) Disable(hostID string) error
func (m *HostManager) Reorder(hostIDs []string)
func (m *HostManager) Snapshot() []HostSnapshot
func (m *HostManager) ClientFor(hostID string) *client.Client
func (m *HostManager) SnapshotCh() <-chan struct{}
```

### Host（單一主機連線封裝）

```go
type Host struct {
    ID       string          // "local" 或 hostname
    Config   HostConfig
    Status   HostStatus      // Connected / Connecting / Disconnected / Disabled
    client   *client.Client
    tunnel   *remote.Tunnel  // 遠端才有
    snapshot *tsmv1.StateSnapshot
    cancel   context.CancelFunc
}

type HostStatus int
const (
    HostDisabled HostStatus = iota
    HostConnecting
    HostConnected
    HostDisconnected
)
```

### 資料流

```
Host goroutine (每個 host 一個):
  loop:
    snapshot := client.RecvSnapshot()
    host.snapshot = snapshot
    hostMgr.snapshotCh <- struct{}{}

UI Cmd:
  <-hostMgr.SnapshotCh()
  return SnapshotMsg(hostMgr.Snapshot())
```

---

## §2 Config 持久化與 CLI 參數整合

### config.toml 新增結構

```toml
[[hosts]]
name = "local"
address = ""
color = "#5B9BD5"
enabled = true
sort_order = 0

[[hosts]]
name = "dev-server"
address = "dev.example.com"
color = "#70AD47"
enabled = true
sort_order = 1

[[hosts]]
name = "staging"
address = "staging.example.com"
color = "#FFC000"
enabled = false
sort_order = 2
```

### Go 結構

```go
type HostEntry struct {
    Name      string `toml:"name"`
    Address   string `toml:"address"`
    Color     string `toml:"color"`
    Enabled   bool   `toml:"enabled"`
    SortOrder int    `toml:"sort_order"`
}
```

### CLI 參數整合邏輯

啟動時：

1. 讀取 config.toml 的 hosts 清單
2. 若有 `--remote` 參數：
   - 參數中的 host 若不在清單 → 自動新增並設為 enabled
   - 參數中的 host 若在清單但 disabled → 設為 enabled
   - 參數中沒有的 host → 維持 config 中的 enabled 狀態
   - `local` 若不在參數中 → 設為 disabled（本次）
3. 若無 `--remote` 參數：
   - `local` 強制 enabled
   - 其他 host 依 config 中的 enabled 狀態
4. 上述 enabled 狀態變更寫回 config.toml

---

## §3 UI 變更

### ListItem 擴展

```go
type ListItem struct {
    Type    ItemType  // 新增 ItemHostTitle
    Session tmux.Session
    Group   store.Group
    HostID  string    // 所有 item 都帶 host 標記
}

const (
    ItemSession  ItemType = iota
    ItemGroup
    ItemHostTitle
)
```

### FlattenMultiHost

遍歷順序（依 host sort_order）：

```
  dev-server   services: 3 sessions
  ▾ services
      nginx          running   2m ago
    monitoring       idle      15m ago
  staging  連線中...
```

### Host Title 樣式

- 文字左右各多一格有背景色（類似 tab badge），非全寬背景
- 斷線時背景色變深灰，連線狀態文字顯示在標題後方（無背景）

### Status Bar 顏色

- 不隨選單游標即時變動
- Attach 進入 session 時套用該 host 的 accent color 作為 status bar 背景色
- 沿用現有 Local/Remote 色彩機制，改從 host entry 的 color 欄位讀取

### 操作轉發

```go
item := m.currentItem()
client := m.hostMgr.ClientFor(item.HostID)
// 用對應的 client 發送 RPC
```

- 本機 attach：tmux switch-client / attach-session
- 遠端 attach：remote.Attach(host.Address, session.Name)

### Host Title 游標行為

- 可被選中，選中時 status bar 顯示主機資訊
- 不可對 host title 行做 session 操作

---

## §4 主機管理面板（`h` 鍵）

### 觸發

主選單按 `h` → 進入 `ModeHostPicker`

### 樣式

```
┌ 主機管理 ──────────────┐
│  ● local          ▲    │
│  ● dev-server     ▼    │
│  ○ staging              │
│                         │
│  a:新增 d:刪除 ↑↓:排序  │
│  space:啟用/停用 esc:返回│
└─────────────────────────┘
```

- `●` 已啟用 / `○` 已停用
- `▲▼` 顯示在游標所在行

### 操作

| 按鍵 | 動作 |
|------|------|
| `↑/↓` 或 `j/k` | 移動游標 |
| `space` | 切換啟用/停用 |
| `a` | 新增主機（輸入 hostname，自動分配顏色） |
| `d` | 刪除主機（確認對話框，local 不可刪除） |
| `shift+↑/↓` 或 `shift+j/k` | 上下移動排序 |
| `esc` | 關閉面板，變更即時生效並寫回 config.toml |

### 即時效果

- 啟用：HostManager.Enable() → 建立連線 → 主選單出現該 host 區塊
- 停用：HostManager.Disable() → 斷線 → 主選單移除該 host 區塊
- local 永遠在清單中，不可刪除，可停用

---

## §5 連線生命週期與重連

### 啟動流程

1. 讀取 config + 套用 `--remote` 參數
2. 建立 HostManager
3. 對每個 enabled host 平行啟動連線
4. UI 不等所有 host 連線完成，立即顯示
5. 已連線的 host 顯示 session 列表，連線中的顯示狀態

### Watch goroutine

```go
func (h *Host) run(ctx context.Context, notifyCh chan<- struct{}) {
    for {
        if err := h.connect(ctx); err != nil {
            h.Status = HostDisconnected
            notifyCh <- struct{}{}
            h.backoff(ctx)  // 1s→2s→4s→8s→16s→30s
            continue
        }
        h.Status = HostConnected
        notifyCh <- struct{}{}

        for {
            snap, err := h.client.RecvSnapshot()
            if err != nil {
                h.Status = HostDisconnected
                notifyCh <- struct{}{}
                break
            }
            h.snapshot = snap
            notifyCh <- struct{}{}
        }
    }
}
```

### Attach 流程

- 本機：現行邏輯不變
- 遠端：`remote.Attach()` → 結束後回到多主機選單
- 斷線由 Host goroutine 自動背景重連，不走 reconnect modal

### 與現有 runRemote 的關係

`runRemote()` 可保留為向下相容捷徑，內部改為建立只含一個 remote host 的 HostManager。

---

## §6 Proto 與資料模型

### Proto 不改動

`StateSnapshot`、`Session`、`Group` 維持不變。Host 維度完全在客戶端處理。

### HostSnapshot（客戶端結構）

```go
type HostSnapshot struct {
    HostID   string
    Name     string
    Color    string
    Status   HostStatus
    Error    string
    Snapshot *tsmv1.StateSnapshot
}
```

### Store（SQLite）不改動

Groups 和 SessionMeta 各 daemon 本地管理，主機清單存 config.toml。

---

## 架構選型

**方案 A: Multi-Client Aggregator**（採用）

在 UI 與 Client 之間新增 HostManager 中間層，管理多個 Client 連線並組合快照。

選擇理由：
- 職責分離：HostManager 管連線，UI 管顯示
- 即時性與單 Client 方案相同（in-process channel 轉發）
- 改動集中，可漸進式開發
- 為遠期 custom 模式留下擴展點
