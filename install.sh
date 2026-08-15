#!/bin/sh
# adastra installer — https://github.com/tawhidkuet04/adastra
# Usage: curl -fsSL https://raw.githubusercontent.com/tawhidkuet04/adastra/main/install.sh | sh
set -eu

REPO="tawhidkuet04/adastra"
INSTALL_DIR="${ADASTRA_INSTALL_DIR:-/usr/local/bin}"

main() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)
  case "$arch" in
    x86_64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac
  case "$os" in
    darwin | linux) ;;
    *) err "unsupported OS: $os (Windows: use 'go install github.com/$REPO@latest' or download from releases)" ;;
  esac

  tag=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  [ -n "$tag" ] || err "could not determine latest release — is the first release published?"

  url="https://github.com/$REPO/releases/download/$tag/adastra_${tag#v}_${os}_${arch}.tar.gz"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  echo "→ downloading adastra $tag ($os/$arch)"
  curl -fsSL "$url" -o "$tmp/adastra.tar.gz" || err "download failed: $url"
  tar -xzf "$tmp/adastra.tar.gz" -C "$tmp"

  if [ -w "$INSTALL_DIR" ]; then
    mv "$tmp/adastra" "$INSTALL_DIR/adastra"
  else
    echo "→ $INSTALL_DIR needs sudo"
    sudo mv "$tmp/adastra" "$INSTALL_DIR/adastra"
  fi
  chmod +x "$INSTALL_DIR/adastra" 2>/dev/null || sudo chmod +x "$INSTALL_DIR/adastra"

  echo "✓ installed: $("$INSTALL_DIR/adastra" --version)"
  echo ""
  echo "next steps:"
  echo "  adastra docs show getting-started"
  echo "  adastra auth login --client-id ... --team-id ... --key-id ... --private-key ./key.pem"
}

err() {
  echo "error: $1" >&2
  exit 1
}

main "$@"
