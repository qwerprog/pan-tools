#!/usr/bin/env sh
set -eu

REPO="qwerprog/pan-tools"
CACHE_DIR="${XDG_CACHE_HOME:-${HOME:-/tmp}/.cache}/pan-tools/bin"
CACHED_BIN="$CACHE_DIR/pant"

resolve_pant() {
  if [ -n "${PANT_BIN:-}" ] && [ -x "$PANT_BIN" ]; then
    printf '%s\n' "$PANT_BIN"
    return 0
  fi

  if found=$(command -v pant 2>/dev/null); then
    printf '%s\n' "$found"
    return 0
  fi

  if command -v go >/dev/null 2>&1; then
    gopath=$(go env GOPATH 2>/dev/null || true)
    if [ -n "$gopath" ] && [ -x "$gopath/bin/pant" ]; then
      printf '%s\n' "$gopath/bin/pant"
      return 0
    fi
  fi

  if [ -n "${HOME:-}" ] && [ -x "$HOME/go/bin/pant" ]; then
    printf '%s\n' "$HOME/go/bin/pant"
    return 0
  fi

  if [ -n "${HOME:-}" ] && [ -x "$HOME/.local/bin/pant" ]; then
    printf '%s\n' "$HOME/.local/bin/pant"
    return 0
  fi

  if [ -x "$CACHED_BIN" ]; then
    printf '%s\n' "$CACHED_BIN"
    return 0
  fi

  script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
  repo_bin="$script_dir/../../../../../pant"
  if [ -x "$repo_bin" ]; then
    printf '%s\n' "$repo_bin"
    return 0
  fi

  return 1
}

fetch_text() {
  url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" 2>/dev/null
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$url" 2>/dev/null
  else
    return 1
  fi
}

download_file() {
  url="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$dest" "$url"
  else
    return 1
  fi
}

resolve_latest_tag() {
  tag=""
  if command -v curl >/dev/null 2>&1; then
    tag=$(curl -sI "https://github.com/$REPO/releases/latest" 2>/dev/null | grep -i '^location:' | sed 's/.*tag\///' | tr -d '\r\n' || true)
  fi
  if [ -z "$tag" ]; then
    release_json=$(fetch_text "https://api.github.com/repos/$REPO/releases/latest" || true)
    tag=$(printf '%s\n' "$release_json" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | head -n 1 || true)
  fi
  printf '%s\n' "$tag"
}

download_release() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)
  case "$os" in
    linux|darwin) ;;
    *) return 1 ;;
  esac
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) return 1 ;;
  esac

  if [ "$os" = "linux" ]; then
    ext="tar.gz"
  else
    ext="zip"
  fi

  tag=$(resolve_latest_tag)
  [ -n "$tag" ] || return 1

  dl_url="https://github.com/$REPO/releases/download/${tag}/pant-${tag}-${os}-${arch}.${ext}"

  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT
  archive_file="$tmp_dir/pant.$ext"
  download_file "$dl_url" "$archive_file" || return 1

  mkdir -p "$tmp_dir/unpack"
  if [ "$ext" = "tar.gz" ]; then
    tar -xzf "$archive_file" -C "$tmp_dir/unpack" || return 1
  elif command -v unzip >/dev/null 2>&1; then
    unzip -qo "$archive_file" -d "$tmp_dir/unpack" || return 1
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" "$archive_file" "$tmp_dir/unpack" || return 1
  else
    return 1
  fi

  extracted=$(find "$tmp_dir/unpack" -maxdepth 3 -type f -name 'pant' | head -n 1 || true)
  [ -n "$extracted" ] || return 1

  mkdir -p "$CACHE_DIR"
  cp "$extracted" "$CACHED_BIN.tmp"
  chmod +x "$CACHED_BIN.tmp"
  mv "$CACHED_BIN.tmp" "$CACHED_BIN"
  printf '%s\n' "$CACHED_BIN"
}

if ! pant_bin=$(resolve_pant); then
  if ! pant_bin=$(download_release); then
    cat >&2 <<'EOF'
Error: `pant` binary not found and automatic download from GitHub Releases failed.
Install `pant` via one of the following methods (or set PANT_BIN=/path/to/pant):
  - One-liner: curl -fsSL https://raw.githubusercontent.com/qwerprog/pan-tools/main/install.sh | bash
  - Homebrew:  brew install qwerprog/pan-tools/pant
  - Arch AUR:  yay -S pan-tools-bin
  - Releases:  https://github.com/qwerprog/pan-tools/releases
EOF
    exit 127
  fi
fi

exec "$pant_bin" "$@"
