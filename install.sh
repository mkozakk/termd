#!/bin/sh
set -e

REPO="mkozakk/termd"
PLATFORM=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
fi

echo "Detecting latest version..."
TAG=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$TAG" ] || [ "$TAG" = "null" ]; then
    echo "Error: Could not find latest release for $REPO"
    exit 1
fi

URL="https://github.com/$REPO/releases/download/$TAG/termd_${PLATFORM}_${ARCH}.tar.gz"

echo "Downloading termd $TAG for ${PLATFORM}/${ARCH}..."
curl -L "$URL" | tar -xz

chmod +x termd

if [ -w "/usr/local/bin" ]; then
    mv termd /usr/local/bin/termd
    echo "Installed to /usr/local/bin/termd"
else
    echo "Cannot write to /usr/local/bin. Trying sudo..."
    if command -v sudo >/dev/null 2>&1; then
        sudo mv termd /usr/local/bin/termd
        echo "Installed to /usr/local/bin/termd (via sudo)"
    else
        mkdir -p "$HOME/.local/bin"
        mv termd "$HOME/.local/bin/termd"
        echo "Installed to $HOME/.local/bin/termd"
        echo "Please ensure $HOME/.local/bin is in your PATH"
    fi
fi
