#!/bin/sh
set -eu

REPO="khang859/rune"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

info() {
  printf '%s\n' "$*"
}

err() {
  printf 'error: %s\n' "$*" >&2
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "required command not found: $1"
    exit 1
  fi
}

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    err "unsupported OS: $(uname -s)"
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *)
    err "unsupported architecture: $(uname -m)"
    exit 1
    ;;
esac

asset="rune-${os}-${arch}"
url="${BASE_URL}/${asset}"

if [ -n "${RUNE_INSTALL_DIR:-}" ]; then
  install_dir="$RUNE_INSTALL_DIR"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_dir="/usr/local/bin"
else
  install_dir="${HOME}/.local/bin"
fi

dest="${install_dir}/rune"

if command -v curl >/dev/null 2>&1; then
  downloader="curl"
elif command -v wget >/dev/null 2>&1; then
  downloader="wget"
else
  err "curl or wget is required"
  exit 1
fi

need_cmd uname
need_cmd mktemp
need_cmd chmod
need_cmd mkdir
need_cmd mv

mkdir -p "$install_dir"

if [ ! -w "$install_dir" ]; then
  err "install directory is not writable: $install_dir"
  err "set RUNE_INSTALL_DIR to a writable directory, for example:"
  err "  RUNE_INSTALL_DIR=\$HOME/.local/bin sh install.sh"
  exit 1
fi

current=""
if [ -x "$dest" ]; then
  current="$($dest --version 2>/dev/null || true)"
fi

tmp="$(mktemp "${TMPDIR:-/tmp}/rune.XXXXXX")"
trap 'rm -f "$tmp"' EXIT INT TERM

info "Downloading ${asset} from latest release..."
if [ "$downloader" = "curl" ]; then
  if ! curl -fL --progress-bar "$url" -o "$tmp"; then
    err "failed to download $url"
    err "make sure a GitHub release exists with asset: $asset"
    exit 1
  fi
else
  if ! wget -q --show-progress -O "$tmp" "$url"; then
    err "failed to download $url"
    err "make sure a GitHub release exists with asset: $asset"
    exit 1
  fi
fi

chmod +x "$tmp"
mv "$tmp" "$dest"
trap - EXIT INT TERM

installed="$($dest --version 2>/dev/null || printf 'rune installed')"

if [ -n "$current" ]; then
  info "Updated rune:"
  info "  from: $current"
  info "  to:   $installed"
else
  info "Installed rune: $installed"
fi
info "Location: $dest"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    info ""
    info "Note: $install_dir is not in your PATH."
    info "Add it to your shell profile, for example:"
    info "  export PATH=\"$install_dir:\$PATH\""
    ;;
esac

info ""
info "Next steps:"
info "  rune"
info "  # then choose/configure a provider with /providers or /settings"
info "  # Codex users can run: rune login codex"
info ""
info "To update later, rerun this installer."
