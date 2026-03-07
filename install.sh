#!/bin/sh
set -e

REPO="wake/tmux-session-menu"
INSTALL_DIR="$HOME/.local/bin"
BINARY="tsm"

# 偵測作業系統
detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux" ;;
    *)      echo "unsupported"; return 1 ;;
  esac
}

# 偵測 CPU 架構
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)       echo "amd64" ;;
    arm64|aarch64)      echo "arm64" ;;
    *)                  echo "unsupported"; return 1 ;;
  esac
}

# 取得最新 release 版本 tag
latest_version() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//'
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//'
  else
    echo "Error: curl 或 wget 皆未安裝" >&2
    exit 1
  fi
}

# 下載檔案
download() {
  url="$1"; dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$dest" "$url"
  else
    wget -qO "$dest" "$url"
  fi
}

main() {
  os=$(detect_os)
  arch=$(detect_arch)
  echo "偵測到平台: ${os}/${arch}"

  echo "查詢最新版本..."
  tag=$(latest_version)
  if [ -z "$tag" ]; then
    echo "Error: 無法取得最新版本，請確認 GitHub release 已建立" >&2
    exit 1
  fi
  echo "最新版本: ${tag}"

  asset="${BINARY}-${os}-${arch}"
  url="https://github.com/${REPO}/releases/download/${tag}/${asset}"

  echo "下載 ${url} ..."
  tmp=$(mktemp)
  download "$url" "$tmp"
  chmod +x "$tmp"

  mkdir -p "$INSTALL_DIR"
  mv "$tmp" "${INSTALL_DIR}/${BINARY}"
  echo "已安裝到 ${INSTALL_DIR}/${BINARY}"

  # 檢查 PATH
  case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
      echo ""
      echo "注意: ${INSTALL_DIR} 不在 PATH 中"
      echo "請將以下內容加入 ~/.zshrc 或 ~/.bashrc:"
      echo ""
      echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
      echo ""
      ;;
  esac

  echo ""
  echo "執行 tsm setup 進行初始設定..."
  "${INSTALL_DIR}/${BINARY}" setup
}

main
