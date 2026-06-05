#!/bin/bash
# ais Auto-Installation Script
# Downloads latest release from GitHub and installs to ~/.local/bin
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/keepmind9/ai-switch/main/scripts/install.sh | bash
#
# With proxy:
#   curl -sL ... | bash -s -- --proxy http://127.0.0.1:10808

set -e

REPO="keepmind9/ai-switch"
BINARY="ais"
INSTALL_DIR="$HOME/.local/bin"
CURL_PROXY=""

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --proxy)
            CURL_PROXY="$2"
            shift 2
            ;;
        *)
            shift
            ;;
    esac
done

# Auto-detect proxy from environment if not specified
if [ -z "$CURL_PROXY" ]; then
    if [ -n "$https_proxy" ]; then
        CURL_PROXY="$https_proxy"
    elif [ -n "$HTTPS_PROXY" ]; then
        CURL_PROXY="$HTTPS_PROXY"
    elif [ -n "$http_proxy" ]; then
        CURL_PROXY="$http_proxy"
    elif [ -n "$HTTP_PROXY" ]; then
        CURL_PROXY="$HTTP_PROXY"
    fi
fi

# Build curl flags as an array (avoids word-splitting issues)
CURL_FLAGS=(-sL)
if [ -n "$CURL_PROXY" ]; then
    CURL_FLAGS+=(-x "$CURL_PROXY")
fi

echo "Checking ais installation..."

# Get latest version info early
echo "Fetching latest release..."
RELEASE=$(curl "${CURL_FLAGS[@]}" -sf "https://api.github.com/repos/${REPO}/releases/latest")
LATEST_VERSION=$(echo "$RELEASE" | grep -o '"tag_name": "[^"]*"' | head -1 | sed 's/.*: "//;s/"//')

if [ -z "$LATEST_VERSION" ]; then
    echo "Failed to fetch release info. Install manually:"
    echo "  https://github.com/${REPO}/releases"
    exit 1
fi

if command -v "$BINARY" &> /dev/null; then
    CURRENT=$("$BINARY" version 2>/dev/null | grep "^Version:" | awk '{print $2}')
    if [ "$CURRENT" = "${LATEST_VERSION#v}" ]; then
        echo "ais is already up to date ($LATEST_VERSION)."
        exit 0
    fi
    if [ -n "$CURRENT" ]; then
        echo "ais $CURRENT installed, upgrading to $LATEST_VERSION..."
    else
        echo "ais installed, upgrading to $LATEST_VERSION..."
    fi
else
    echo "ais not found. Installing $LATEST_VERSION..."
fi

# Check if ais is currently running (cannot replace a running binary)
if pgrep -x "$BINARY" > /dev/null 2>&1; then
    echo "Error: ais is currently running and cannot be replaced."
    echo ""
    echo "Please stop it first, then re-run this script:"
    echo "  ais stop"
    echo ""
    echo "Or kill the process:"
    echo "  pkill -x $BINARY"
    exit 1
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

echo "Downloading ais ${LATEST_VERSION} for ${OS}/${ARCH}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if ! curl "${CURL_FLAGS[@]}" --connect-timeout 30 "$DOWNLOAD_URL" -o "$TMPDIR/$FILENAME"; then
    echo "Download failed."
    if [ -z "$CURL_PROXY" ]; then
        echo "Hint: if you are behind a firewall, try with proxy:"
        echo "  curl -sL ... | bash -s -- --proxy http://127.0.0.1:10808"
    fi
    echo "Or install manually:"
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
echo "ais ${LATEST_VERSION} installed successfully!"
echo "  Location: $INSTALL_DIR/$BINARY"
echo ""
echo "Verify:"
echo "  ais version"
echo ""
echo "If 'ais' not found, restart your shell or run:"
echo "  source ~/.zshrc  # or source ~/.bashrc"
