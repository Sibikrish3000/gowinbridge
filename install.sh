#!/bin/sh
set -e

# GoWinBridge Installer
# Installs 'winrun' from the latest GitHub release.

REPO="sibikrish3000/gowinbridge"
BINARY="winrun"

# Detection
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$OS" != "linux" ]; then
    echo "Error: This installer is for Linux (WSL) only."
    exit 1
fi

case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "Error: Unsupported architecture $ARCH"
        exit 1
        ;;
esac

# Determine installation directory
if [ -d "$HOME/.local/bin" ] && echo "$PATH" | grep -q "$HOME/.local/bin"; then
    INSTALL_DIR="$HOME/.local/bin"
    USE_SUDO=""
else
    INSTALL_DIR="/usr/local/bin"
    if [ "$(id -u)" -ne 0 ]; then
        USE_SUDO="sudo"
    fi
fi

# Fetch latest release URL
echo "Detecting latest release..."
LATEST_URL=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep "browser_download_url" | grep "linux-$ARCH" | cut -d '"' -f 4)

if [ -z "$LATEST_URL" ]; then
    # Fallback to constructing URL for latest tag if API fails/hit limits
    VERSION=$(curl -sL -o /dev/null -w %{url_effective} "https://github.com/$REPO/releases/latest" | rev | cut -d'/' -f1 | rev)
    LATEST_URL="https://github.com/$REPO/releases/download/$VERSION/winrun-linux-$ARCH"
    echo "Using version: $VERSION"
fi

echo "Downloading $BINARY from $LATEST_URL..."
TMP_FILE=$(mktemp)
curl -sL "$LATEST_URL" -o "$TMP_FILE"

echo "Installing to $INSTALL_DIR/$BINARY..."
chmod +x "$TMP_FILE"
$USE_SUDO mv "$TMP_FILE" "$INSTALL_DIR/$BINARY"

echo "Successfully installed $BINARY to $INSTALL_DIR/$BINARY"
echo "Run '$BINARY --help' to get started."
