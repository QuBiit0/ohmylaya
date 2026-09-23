#!/bin/sh
# ohmylaya one-shot installer for Linux and macOS.
# Usage: curl -fsSL https://raw.githubusercontent.com/QuBiit0/ohmylaya/main/install.sh | sh
# Environment: OHMYLAYA_BIN_DIR (default ~/.local/bin), OHMYLAYA_VERSION (default latest),
# OHMYLAYA_INSTALL_ARGS (extra flags for `ohmylaya install`, default "--yes").
set -eu

REPO="QuBiit0/ohmylaya"
BIN_DIR="${OHMYLAYA_BIN_DIR:-$HOME/.local/bin}"
VERSION="${OHMYLAYA_VERSION:-latest}"
INSTALL_ARGS="${OHMYLAYA_INSTALL_ARGS:---yes}"

say() { printf '%s\n' "$*"; }
die() { printf 'ohmylaya: %s\n' "$*" >&2; exit 1; }

fetch() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
  else die "curl or wget is required"; fi
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$os" in linux|darwin) ;; *) die "unsupported OS: $os" ;; esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac
[ "$os" = darwin ] && [ "$arch" = amd64 ] && die "macOS Intel is not supported; the engine only ships for Apple Silicon"

if [ "$VERSION" = latest ]; then
  tag=$(fetch_tag=$(mktemp); fetch "https://api.github.com/repos/$REPO/releases/latest" "$fetch_tag"; sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$fetch_tag" | head -n1; rm -f "$fetch_tag")
  [ -n "$tag" ] || die "could not determine the latest release"
else
  tag="$VERSION"
fi
version=${tag#v}
archive="ohmylaya_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
say "Downloading ohmylaya $tag for $os/$arch"
fetch "$base/$archive" "$tmp/$archive"
fetch "$base/checksums.txt" "$tmp/checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$expected" ] || die "checksums.txt has no entry for $archive"
if command -v sha256sum >/dev/null 2>&1; then actual=$(sha256sum "$tmp/$archive" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then actual=$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)
else die "sha256sum or shasum is required"; fi
[ "$expected" = "$actual" ] || die "checksum mismatch for $archive: expected $expected, got $actual"

mkdir -p "$BIN_DIR"
tar -xzf "$tmp/$archive" -C "$tmp" ohmylaya
install -m 0755 "$tmp/ohmylaya" "$BIN_DIR/ohmylaya"
say "Installed $BIN_DIR/ohmylaya"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) say "Add $BIN_DIR to your PATH, for example:  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

# shellcheck disable=SC2086
exec "$BIN_DIR/ohmylaya" install $INSTALL_ARGS
