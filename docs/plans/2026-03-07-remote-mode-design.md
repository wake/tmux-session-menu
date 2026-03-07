# tsm --remote 設計文件

## 概述

透過 SSH tunnel 連線到遠端主機的 tsm-daemon，在本地操作遠端 tmux session。
斷線時自動重連並接回原本的 session，免去重新 SSH + 找 session 的麻煩。

## 命令介面

```bash
tsm --remote <ssh-host>
```

`<ssh-host>` 依賴 `~/.ssh/config` 解析（支援 alias、User、Port 等）。

## 前置條件

- 遠端主機必須安裝 tsm 且 tsm-daemon 正在執行
- 遠端未啟動 daemon 時顯示錯誤退出：`遠端主機 <host> 的 tsm-daemon 未啟動`
- OpenSSH 6.7+（unix socket forwarding 支援）

## 生命週期

```
1. 建立 SSH tunnel
   ssh -N -L /tmp/tsm-<hash>.sock:<remote-data-dir>/tsm.sock <host>

2. gRPC 連線到本地 tunnel socket → DaemonStatus 探測（2s timeout）
   失敗 → 錯誤退出

3. TUI session 選單（重用現有 ui.Model）
   ├─ 選取 session → ssh -t <host> tmux attach -t <name>
   │   ├─ exit 0 (detach) → 回到選單
   │   └─ exit != 0 (斷線) → 重連 modal
   │       ├─ 重連成功 + session 存在 → 自動 reattach
   │       ├─ 重連成功 + session 不存在 → 回到選單
   │       └─ 使用者按 Esc → 回到選單 / 按 q → 退出
   ├─ q / esc → 退出 tsm
   └─ ctrl+e → 退出 tsm

4. 清理：kill SSH tunnel 程序、刪除暫存 socket 檔案
```

## SSH Tunnel 架構

使用 Unix socket forwarding（方案 A），遠端 daemon 不需任何修改。

```
N (本地)                                S (遠端)
ssh -N -L                               tsm-daemon
  /tmp/tsm-<hash>.sock                    ~/.config/tsm/tsm.sock
  ─────────────────────────────────────►
gRPC client → /tmp/tsm-<hash>.sock
```

暫存 socket 路徑使用 host 的 hash 避免多個 remote 連線衝突。

## 重連 Modal

斷線時在終端中央顯示 modal：

```
+-----------------------------------+
|                                   |
|   x 連線中斷  user@host           |  紅色（短暫顯示）
|   o 重新連線中... (第 2 次)        |  黃色 + 動畫
|                                   |
|   [Esc] 回到選單  [q] 退出        |
|                                   |
+-----------------------------------+
```

三態 icon：
- 斷線：紅色
- 連線中：黃色（動畫）
- 已連線：綠色（短暫顯示後自動進入 tmux）

重連策略：指數退避 1s → 2s → 4s → 8s → 16s → 30s（上限）。

重連流程：
1. 重建 SSH tunnel
2. gRPC 驗證 daemon 存活
3. 確認目標 session 是否還在
4. 成功 → 重新 spawn `ssh -t host tmux attach -t <name>`
5. session 不存在 → 回到選單

## 新增檔案

| 路徑 | 職責 |
|------|------|
| `internal/remote/tunnel.go` | SSH tunnel 管理（啟動 ssh -N -L、監控程序、清理） |
| `internal/remote/attach.go` | 遠端 session attach（spawn ssh -t、偵測 exit code） |
| `internal/remote/reconnect.go` | 重連迴圈（指數退避、狀態回報） |
| `internal/remote/tunnel_test.go` | tunnel 單元測試 |
| `internal/remote/attach_test.go` | attach 單元測試 |
| `internal/remote/reconnect_test.go` | reconnect 單元測試 |

## 修改檔案

| 路徑 | 變更 |
|------|------|
| `cmd/tsm/main.go` | 新增 `--remote <host>` 參數解析、`runRemote()` 入口函式 |
| `internal/client/client.go` | 新增 `DialSocket(path)` 連線到指定 unix socket（不觸發 auto-start） |

## 不動的部分

- `internal/ui/` — TUI 選單完全重用
- `internal/daemon/` — 遠端 daemon 不需修改
- `api/tsm/v1/` — gRPC proto 不需修改
- `internal/config/` — 設定結構不需修改
