#!/bin/bash
# ai-switch Auto-Installation Script
# Downloads latest release from GitHub and installs to ~/.local/bin

set -e

REPO="keepmind9/ai-switch"
BINARY="ai-switch"
INSTALL_DIR="$HOME/.local/bin"

echo "Checking ai-switch installation..."

# Get latest version info early
echo "Fetching latest release..."
RELEASE=$(curl -sf "https://api.github.com/repos/${REPO}/releases/latest")
LATEST_VERSION=$(echo "$RELEASE" | grep -o '"tag_name": "[^"]*"' | head -1 | sed 's/.*: "//;s/"//')

if [ -z "$LATEST_VERSION" ]; then
    echo "Failed to fetch release info. Install manually:"
    echo "  https://github.com/${REPO}/releases"
    exit 1
fi

if command -v "$BINARY" &> /dev/null; then
    CURRENT=$("$BINARY" version 2>/dev/null | grep "^Version:" | awk '{print $2}')
    if [ "$CURRENT" = "${LATEST_VERSION#v}" ]; then
        echo "ai-switch is already up to date ($LATEST_VERSION)."
        exit 0
    fi
    if [ -n "$CURRENT" ]; then
        echo "ai-switch $CURRENT installed, upgrading to $LATEST_VERSION..."
    else
        echo "ai-switch installed, upgrading to $LATEST_VERSION..."
    fi
else
    echo "ai-switch not found. Installing $LATEST_VERSION..."
fi

# Detect platform
OS=""
ARCH=""
case "$(uname -s)" in
    Linux*)  OS="linux";;
    Darwin*) OS="darwin";;
    *)       echo "Unsupported OS: $(uname -s)"; exit 1;;
esac

case "$(uname -m)" in
    x86_64|amd64)   ARCH="amd64";;
    aarch64|arm64)  ARCH="arm64";;
    *)              echo "Unsupported architecture: $(uname -m)"; exit 1;;
esac

PATTERN="${OS}-${ARCH}"

# Find matching asset download URL
DOWNLOAD_URL=$(echo "$RELEASE" | grep -o "\"browser_download_url\": \"[^\"]*${PATTERN}[^\"]*\"" | head -1 | sed 's/.*: "\(.*\)"/\1/')

if [ -z "$DOWNLOAD_URL" ]; then
    echo "No matching release found for ${PATTERN}."
    echo "Available assets:"
    echo "$RELEASE" | grep "browser_download_url" | sed 's/.*: "//;s/"//'
    exit 1
fi

FILENAME=$(basename "$DOWNLOAD_URL")

echo "Downloading ai-switch ${LATEST_VERSION} for ${OS}/${ARCH}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if ! curl -sL "$DOWNLOAD_URL" -o "$TMPDIR/$FILENAME"; then
    echo "Download failed. Install manually:"
    echo "  https://github.com/${REPO}/releases"
    exit 1
fi

echo "Extracting..."
cd "$TMPDIR"

if [[ "$FILENAME" == *.tar.gz ]]; then
    tar -xzf "$FILENAME"
elif [[ "$FILENAME" == *.zip ]]; then
    unzip -q "$FILENAME"
else
    echo "Unknown archive format: $FILENAME"
    exit 1
fi

# Find the binary (may be in a subdirectory)
BINARY_PATH=$(find . -name "$BINARY" -o -name "${BINARY}.exe" | head -1)

if [ -z "$BINARY_PATH" ]; then
    echo "Binary not found in archive."
    exit 1
fi

mkdir -p "$INSTALL_DIR"
mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY"
chmod +x "$INSTALL_DIR/$BINARY"

# Add to PATH if needed
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "Adding $INSTALL_DIR to PATH..."
    SHELL_RC=""
    if [ -f "$HOME/.zshrc" ]; then
        SHELL_RC="$HOME/.zshrc"
    elif [ -f "$HOME/.bashrc" ]; then
        SHELL_RC="$HOME/.bashrc"
    fi

    if [ -n "$SHELL_RC" ]; then
        echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$SHELL_RC"
    fi
    export PATH="$INSTALL_DIR:$PATH"
fi

echo ""
echo "ai-switch ${LATEST_VERSION} installed successfully!"
echo "  Location: $INSTALL_DIR/$BINARY"
echo ""
echo "Verify:"
echo "  ai-switch version"
echo ""
echo "If 'ai-switch' not found, restart your shell or run:"
echo "  source ~/.zshrc  # or source ~/.bashrc"
