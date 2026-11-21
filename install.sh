#!/usr/bin/env bash
# WhatsFuwa installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/fuwanime/whatsfuwa/main/install.sh | bash
#
# What it does:
#   1. Clones the source to ~/.whatsfuwa/src (or pulls the latest if present).
#   2. Builds the whatsfuwa binary into ~/.local/bin.
#   3. Adds ~/.local/bin to your shell's PATH if it is missing.
#
# Optional environment variables:
#   WHATSFUWA_HOME     install root (default: ~/.whatsfuwa)
#   WHATSFUWA_BIN_DIR  folder for the binary (default: ~/.local/bin)
#   WHATSFUWA_REPO     git URL to install from (default: the official repo)

set -euo pipefail

REPO_URL="${WHATSFUWA_REPO:-https://github.com/fuwanime/whatsfuwa.git}"
INSTALL_HOME="${WHATSFUWA_HOME:-$HOME/.whatsfuwa}"
SRC_DIR="$INSTALL_HOME/src"
BIN_DIR="${WHATSFUWA_BIN_DIR:-$HOME/.local/bin}"
BIN_PATH="$BIN_DIR/whatsfuwa"
MARKER="# added by WhatsFuwa installer"

info() { printf '\033[36m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 belum terpasang. $2"
}

require git "Pasang git terlebih dahulu, lalu jalankan lagi installer ini."
require go "Pasang Go 1.26 atau lebih baru dari https://go.dev/dl/, lalu jalankan lagi installer ini."

mkdir -p "$INSTALL_HOME"
if [ -d "$SRC_DIR/.git" ]; then
  info "Memperbarui source di $SRC_DIR"
  git -C "$SRC_DIR" pull --ff-only || fail "git pull gagal di $SRC_DIR"
else
  if [ -e "$SRC_DIR" ]; then
    fail "$SRC_DIR sudah ada tetapi bukan checkout WhatsFuwa. Hapus folder itu atau atur WHATSFUWA_HOME."
  fi
  info "Mengunduh source ke $SRC_DIR"
  git clone --depth 1 "$REPO_URL" "$SRC_DIR"
fi

info "Membangun WhatsFuwa (ini bisa memakan waktu beberapa menit saat pertama kali)..."
mkdir -p "$BIN_DIR"
# Build to a temporary name first so a running copy is never left half-written.
(cd "$SRC_DIR" && go build -trimpath -ldflags="-s -w" -o "$BIN_PATH.new" ./cmd/whatsfuwa) \
  || fail "build gagal. Periksa versi Go (butuh 1.26 atau lebih baru) dan coba lagi."
mv -f "$BIN_PATH.new" "$BIN_PATH"
chmod +x "$BIN_PATH"

# Add BIN_DIR to PATH for future shells, once.
add_path_line() {
  local rc="$1"
  local line="export PATH=\"$BIN_DIR:\$PATH\""
  touch "$rc"
  if grep -qF "$MARKER" "$rc"; then
    return 0
  fi
  printf '\n%s\n%s\n' "$MARKER" "$line" >> "$rc"
  info "PATH ditambahkan ke $rc"
}

PATH_READY=0
case ":$PATH:" in
  *":$BIN_DIR:"*) PATH_READY=1 ;;
esac

if [ "$PATH_READY" -eq 0 ]; then
  case "${SHELL:-}" in
    */zsh)
      add_path_line "$HOME/.zshrc"
      ;;
    */fish)
      info "Shell fish terdeteksi. Jalankan sekali: fish_add_path $BIN_DIR"
      ;;
    *)
      if [ "$(uname -s)" = "Darwin" ]; then
        add_path_line "$HOME/.bash_profile"
      else
        add_path_line "$HOME/.bashrc"
      fi
      ;;
  esac
fi

echo
info "WhatsFuwa terpasang di $BIN_PATH"
if [ "$PATH_READY" -eq 0 ]; then
  echo "Buka terminal baru, atau jalankan sekarang:"
  echo "  export PATH=\"$BIN_DIR:\$PATH\""
fi
echo "Lalu jalankan:  whatsfuwa"
echo "Perbarui kapan saja dengan:  whatsfuwa update"
