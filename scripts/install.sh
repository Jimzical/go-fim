#!/bin/bash
# go-fim Download Script
# Usage: curl -sSL https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.sh | bash
# Downloads go-fim binary to the current directory

set -e

REPO="Jimzical/go-fim"
VERSION="${1:-latest}"

# Detect platform
case "$(uname -s)" in
    Linux*)  OS="linux" ;;
    Darwin*) OS="darwin" ;;
    *)       echo "Unsupported OS"; exit 1 ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             echo "Unsupported arch"; exit 1 ;;
esac

# Get latest version if needed
if [ "$VERSION" = "latest" ]; then
    VERSION=$(curl -sI "https://github.com/${REPO}/releases/latest" | grep -i "location:" | sed 's/.*\/v//' | tr -d '\r\n')
    [ -z "$VERSION" ] && echo "Failed to fetch version" && exit 1
fi

echo "Downloading go-fim v${VERSION} for ${OS}_${ARCH}..."

# Download and extract
URL="https://github.com/${REPO}/releases/download/v${VERSION}/go-fim_${VERSION}_${OS}_${ARCH}.tar.gz"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

curl -fsSL -o "$TMP/go-fim.tar.gz" "$URL" || { echo "Download failed"; exit 1; }
tar -xzf "$TMP/go-fim.tar.gz" -C "$TMP"
mv "$TMP/go-fim" ./go-fim
chmod +x ./go-fim

echo "Done! Downloaded: ./go-fim"
