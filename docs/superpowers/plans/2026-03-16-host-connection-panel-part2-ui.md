# Host Connection Panel Part 2: UI 拆分 + 連線欄 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 將 `hostpicker.go`（1254 行）拆成三個檔案，加入三欄焦點模型和連線面板（hostcheck 整合、[r]/[t]/[s] 操作），簡化 [i] 面板為唯讀。

**Architecture:** `hostpicker.go` 保留左欄列表 + 三欄編排。設定面板搬到 `hostsettings.go`。新建 `hostconnection.go` 實作連線欄。`hostFocusCol` 取代 `hostPanelOpen`。hub/非 hub 雙路徑保留但中欄/右欄內部統一。

**Tech Stack:** Go 1.24 / Bubble Tea / lipgloss / testify

**Spec:** `docs/superpowers/specs/2026-03-16-host-connection-panel-design.md`

---

## File Structure

| 檔案 | 職責 | 動作 |
|------|------|------|
| `internal/ui/hostpicker.go` | 三欄編排、焦點管理、左欄列表 | 重構（大幅縮減） |
| `internal/ui/hostsettings.go` | 右欄：設定面板 update + render | 新建（從 hostpicker.go 搬出） |
| `internal/ui/hostconnection.go` | 中欄：連線狀態 update + render + [r]/[t]/[s] | 新建 |
| `internal/ui/hostpicker_test.go` | 三欄焦點切換測試 | 修改 |
| `internal/ui/hostsettings_test.go` | 設定面板測試 | 新建（從 hostpicker_test.go 搬出） |
| `internal/ui/hostconnection_test.go` | 連線欄測試 | 新建 |
| `internal/ui/app.go` | Model 欄位：hostFocusCol 取代 hostPanelOpen | 修改 |
| `internal/ui/info.go` | 移除 hub 連線輸入，簡化為唯讀 | 修改 |
| `internal/ui/info_test.go` | 更新測試 | 修改 |

---

## Chunk 1: 焦點模型 + 檔案拆分（無新功能）

目標：拆完檔案、換焦點模型，行為和現有完全一致（中欄暫時為空佔位）。

### Task 1: hostFocusCol 取代 hostPanelOpen

**Files:**
- Modify: `internal/ui/app.go` (Model 欄位)
- Modify: `internal/ui/hostpicker.go` (所有 hostPanelOpen 引用)

- [ ] **Step 1: 在 app.go 加 hostFocusCol，保留 hostPanelOpen 為 computed**

在 `internal/ui/app.go` 的 Model struct，將 `hostPanelOpen bool` 替換：

```go
hostFocusCol     int  // 0=左欄, 1=中欄(連線), 2=右欄(設定)
```

加一個 computed 方法讓現有程式碼過渡：

```go
func (m Model) hostPanelOpen() bool { return m.hostFocusCol > 0 }
```

將原本 `m.hostPanelOpen = true` 改為 `m.hostFocusCol = 2`（直接到設定欄，保持現有行為），`m.hostPanelOpen = false` 改為 `m.hostFocusCol = 0`。

- [ ] **Step 2: 全域搜尋替換 hostPanelOpen 引用**

搜尋 `hostpicker.go` 中所有 `m.hostPanelOpen`：
- `m.hostPanelOpen = true` → `m.hostFocusCol = 2`
- `m.hostPanelOpen = false` → `m.hostFocusCol = 0`
- `m.hostPanelOpen` (讀取) → `m.hostPanelOpen()` (method call)
- `!m.hostPanelOpen` → `!m.hostPanelOpen()`

同樣處理 `app.go` 中的引用。

- [ ] **Step 3: 更新測試的 HostPanelOpen accessor**

```go
func (m Model) HostPanelOpen() bool { return m.hostFocusCol > 0 }
```

- [ ] **Step 4: 執行測試**

```bash
go test ./internal/ui/... -v -race
```

- [ ] **Step 5: Commit**

```bash
git add internal/ui/app.go internal/ui/hostpicker.go
git commit -m "refactor: hostFocusCol 取代 hostPanelOpen — 三欄焦點模型基礎"
```

---

### Task 2: 搬出 hostsettings.go（右欄）

**Files:**
- Create: `internal/ui/hostsettings.go`
- Modify: `internal/ui/hostpicker.go` (移除搬出的函式)

將以下函式從 `hostpicker.go` 搬到新的 `hostsettings.go`：

**非 hub 路徑：**
- `updateHostPanelOpen` (L257-288)
- `toggleHostPanelEnabled` (L289-300)
- `enterHostPanelEdit` (L301-314)
- `updateHostPanelEditing` (L316-342)
- `applyHostDrafts` (L344-361)
- `persistHostsWithSync` (L362-386)
- `renderHostPickerRight` (L526-602)
- `renderColorPreview` (L604-631)

**Hub 路徑：**
- `updateHubHostPanelOpen` (L996-1024)
- `toggleHubHostPanelEnabled` (L1026-1036)
- `enterHubHostPanelEdit` (L1038-1050)
- `updateHubHostPanelEditing` (L1052-1075)
- `applyHubHostDrafts` (L726-743)
- `persistHubHosts` (L745-768)
- `renderHubHostPickerRight` (L1185-end)

**共用：**
- `hostDraftEntry` struct (L19-28)
- `hostPanelFieldCount`, `hostPanelFieldLabels` (L29-40)
- `hexColorRe`, `isValidHexColor` (L41-47)
- `draftFieldValue`, `setDraftField` (L94-122)

- [ ] **Step 1: 建立 hostsettings.go，搬移函式**

新檔案 `package ui`，搬入上述函式。不修改任何邏輯。

- [ ] **Step 2: 從 hostpicker.go 刪除搬出的函式**

- [ ] **Step 3: 執行測試**

```bash
go test ./internal/ui/... -v -race
```

所有測試必須通過（純搬移，不改邏輯）。

- [ ] **Step 4: Commit**

```bash
git add internal/ui/hostsettings.go internal/ui/hostpicker.go
git commit -m "refactor: 搬出 hostsettings.go — 設定面板從 hostpicker 獨立"
```

---

### Task 3: 建立 hostconnection.go 空殼 + 三欄渲染

**Files:**
- Create: `internal/ui/hostconnection.go`
- Modify: `internal/ui/hostpicker.go` (renderHostPicker 改三欄)

- [ ] **Step 1: 建立 hostconnection.go 空殼**

```go
package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateHostConnection 處理中欄（連線設定）的按鍵。
func (m Model) updateHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// TODO: 實作連線欄互動
	switch msg.String() {
	case "left", "h":
		m.hostFocusCol = 0 // 回到左欄，收合
		return m, nil
	case "right", "l":
		m.hostFocusCol = 2 // 到設定欄
		return m, nil
	}
	return m, nil
}

// renderHostConnection 渲染中欄（連線設定）。
func (m Model) renderHostConnection(hostName string, isLocal bool) string {
	var b strings.Builder
	title := fmt.Sprintf("── %s 連線 ──", hostName)
	b.WriteString("  " + selectedStyle.Render(title) + "\n")
	if isLocal {
		b.WriteString("  tmux:  （待實作）\n")
	} else {
		b.WriteString("  SSH:     （待實作）\n")
		b.WriteString("  tmux:    （待實作）\n")
		b.WriteString("  tunnel:  （待實作）\n")
		b.WriteString("  hub-sock:（待實作）\n")
	}
	return b.String()
}

// updateHubHostConnection 處理 hub 模式中欄的按鍵。
func (m Model) updateHubHostConnection(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		m.hostFocusCol = 0
		return m, nil
	case "right", "l":
		m.hostFocusCol = 2
		return m, nil
	}
	return m, nil
}
```

- [ ] **Step 2: 修改 hostpicker.go 的焦點分派**

在 `updateHostPicker` 中，加入 `hostFocusCol == 1` 的分派：

```go
func (m Model) updateHostPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+S 全域
	if msg.String() == "ctrl+s" { ... }

	switch m.hostFocusCol {
	case 0:
		return m.updateHostPickerLeft(msg, hosts)
	case 1:
		return m.updateHostConnection(msg)
	case 2:
		if m.hostPanelEditing {
			return m.updateHostPanelEditing(msg)
		}
		return m.updateHostPanelOpen(msg, hosts)
	}
}
```

同樣修改 `updateHubHostPicker` 的分派。

- [ ] **Step 3: 修改 renderHostPicker 為三欄**

現有：
```go
lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
```

改為：
```go
if m.hostFocusCol > 0 {
    mid := m.renderHostConnection(hostName, isLocal)
    right := m.renderHostPickerRight(hosts)
    return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, mid, right)
}
return leftPanel
```

- [ ] **Step 4: 修改左欄 Enter/→ 改為 focusCol=1（中欄）**

現有 Enter/→ 設定 `hostFocusCol = 2`，改為 `hostFocusCol = 1`。

- [ ] **Step 5: 執行測試**

```bash
go test ./internal/ui/... -v -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/ui/hostconnection.go internal/ui/hostpicker.go
git commit -m "feat: 三欄佈局空殼 — 中欄連線面板佔位"
```

---

### Task 4: 三欄焦點切換測試

**Files:**
- Modify: `internal/ui/hostpicker_test.go`

- [ ] **Step 1: 寫焦點切換測試**

```go
func TestHostPicker_ThreeColumnFocus(t *testing.T) {
	deps := ui.Deps{
		Cfg: config.Default(),
		// 需要 HostMgr 或 HubMode 才能進入 ModeHostPicker
		// 使用現有的測試 setup pattern
	}
	// ... setup model with hosts ...

	// 驗證初始焦點在左欄
	// Enter → 焦點到中欄 (hostFocusCol == 1)
	// → → 焦點到右欄 (hostFocusCol == 2)
	// ← → 焦點回中欄
	// ← → 焦點回左欄，面板收合
	// Esc 從任何位置 → 回左欄
}
```

（具體測試程式碼依現有 `hostpicker_test.go` 的 setup pattern 寫）

- [ ] **Step 2: 執行測試**

```bash
go test ./internal/ui/... -v -race -run TestHostPicker_ThreeColumn
```

- [ ] **Step 3: Commit**

```bash
git add internal/ui/hostpicker_test.go
git commit -m "test: 三欄焦點切換測試"
```

---

## Chunk 2: 連線欄功能實作

### Task 5: hostcheck 整合到連線欄

**Files:**
- Modify: `internal/ui/hostconnection.go`
- Modify: `internal/ui/app.go` (新增 hostcheck 結果到 Model)

- [ ] **Step 1: Model 加 hostcheck 結果欄位**

在 `app.go` 的 Model struct 加：

```go
hostCheckResults map[string]hostcheck.Result // hostID → 檢測結果
```

- [ ] **Step 2: renderHostConnection 顯示真實狀態**

從 `hostCheckResults` 讀取 SSH/tmux 狀態，從 `hubHostSnap` 讀取 tunnel/hub-sock 狀態。渲染 ✓/✗/⠋ spinner。

- [ ] **Step 3: [r] 按鍵觸發 hostcheck**

```go
case "r":
	return m, checkHostCmd(hostName, hostAddr)
```

非同步執行 `hostcheck.Checker.CheckLocal()` 或 `CheckRemote()`，結果透過 `hostCheckResultMsg` 回傳。

- [ ] **Step 4: 寫測試**

- [ ] **Step 5: Commit**

---

### Task 6: [t] 重建 tunnel + [s] 設定 hub-socket

**Files:**
- Modify: `internal/ui/hostconnection.go`

- [ ] **Step 1: [t] 按鍵呼叫 ReconnectHost RPC**

```go
case "t":
	if !isLocal {
		return m, reconnectHostCmd(m.deps.Client, hostID)
	}
```

- [ ] **Step 2: [s] 按鍵進入文字輸入模式**

只接受 `tsm-hub-*` pattern。Enter 後呼叫 `remote.SetHubSocket` + `remote.SetHubSelf`。

- [ ] **Step 3: 寫測試**

- [ ] **Step 4: Commit**

---

### Task 7: [i] 面板簡化為唯讀

**Files:**
- Modify: `internal/ui/info.go`
- Modify: `internal/ui/info_test.go`

- [ ] **Step 1: 移除 hub 連線輸入和 infoConnect**

保留：模式、版本、Daemon socket 路徑顯示。
移除：`infoHubInput`、`hubReconnectSock`、`infoConnect()`、Enter 處理。

- [ ] **Step 2: 更新測試**

移除 `TestInfoMode_HubReconnect`、`TestInfoMode_PersistsHubSocket`、`TestInfoMode_RejectNonHubSocket`。保留 `TestInfoMode_EnterAndExit`、`TestInfoMode_RenderContainsMode`。

- [ ] **Step 3: 移除 runDaemonTUI 的 hubReconnectSock 處理**

在 `cmd/tsm/main.go` 中，`runDaemonTUI` 返回 `hubSock` 的路徑不再需要（[i] 不再提供此功能）。簡化 `runDaemonTUI` 回傳值。

- [ ] **Step 4: Commit**

---

### Task 8: 版本推進

- [ ] **Step 1:** `echo "0.47.0" > VERSION`
- [ ] **Step 2:** 更新 CHANGELOG
- [ ] **Step 3:** `make build && make test`
- [ ] **Step 4:** Commit
