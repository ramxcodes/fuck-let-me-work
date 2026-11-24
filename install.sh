#!/bin/sh
# Installs flmw (fuck let me work) from the latest GitHub release on macOS or Linux.
#   curl -fsSL https://raw.githubusercontent.com/ramxcodes/fuck-let-me-work/main/install.sh | sh
# It downloads the binary, checks its SHA-256 against the release checksums,
# and runs "sudo flmw install", which sets up the service and the local CA.
set -eu

repo="ramxcodes/fuck-let-me-work"

case "$(uname -s)" in
  Darwin) asset="flmw-darwin-universal" ;;
  Linux)
    case "$(uname -m)" in
      x86_64 | amd64) asset="flmw-linux-amd64" ;;
      aarch64 | arm64) asset="flmw-linux-arm64" ;;
      *) echo "No Linux build for $(uname -m)." >&2; exit 1 ;;
    esac ;;
  *) echo "On Windows, download flmw-windows-*.exe from https://github.com/$repo/releases" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
base="https://github.com/$repo/releases/latest/download"

echo "Downloading $asset ..."
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

cd "$tmp"
grep " $asset\$" checksums.txt > expected || { echo "No checksum for $asset in the release." >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum -c expected
else
  shasum -a 256 -c expected
fi

chmod +x "$asset"
echo "Installing (needs your password) ..."
sudo "./$asset" install
