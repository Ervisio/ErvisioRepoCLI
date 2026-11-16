#!/bin/sh
# Installs eg, the ErvisioRepo CLI, from the GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/Ervisio/ErvisioRepoCLI/main/install.sh | sh
#
# Environment:
#   EG_VERSION      version to install (default: latest release), e.g. 0.1.0
#   EG_INSTALL_DIR  target directory (default: /usr/local/bin when writable, else ~/.local/bin)
set -eu

repo="Ervisio/ErvisioRepoCLI"

fail() { echo "eg install: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }

need curl
need tar
need uname

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported system $(uname -s); on Windows use install.ps1" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

version="${EG_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
    sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$version" ] || fail "cannot find the latest release"
fi
version="${version#v}"

name="eg_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/v$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading eg $version for $os/$arch..."
curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz" || fail "download failed: $base/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "checksums download failed"

want=$(grep " $name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || fail "no checksum for $name.tar.gz"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)
else
  got=$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1)
fi
[ "$want" = "$got" ] || fail "checksum mismatch for $name.tar.gz"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"

dir="${EG_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
install -m 0755 "$tmp/$name/eg" "$dir/eg" 2>/dev/null || cp "$tmp/$name/eg" "$dir/eg"
chmod 0755 "$dir/eg"
# Downloaded binaries are quarantined on macOS; eg is not notarized.
if [ "$os" = darwin ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$dir/eg" 2>/dev/null || true
fi

echo "Installed $("$dir/eg" --version | head -n 1) to $dir/eg"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH, e.g.: echo 'export PATH=\"$dir:\$PATH\"' >> ~/.zshrc" ;;
esac
echo "Next: eg auth login --hostname <your-ervisio-host>"
