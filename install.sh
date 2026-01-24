#!/bin/sh
set -e

# Dumber installer script
# Usage: curl -fsSL https://dumber.bnema.dev/install | bash

REPO="bnema/dumber"

# Prefer ~/.local/bin if it exists, otherwise /usr/local/bin
if [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
else
    INSTALL_DIR="/usr/local/bin"
fi

echo "Installing Dumber..."

# Check OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo "Error: Only Linux is supported. Detected: $OS"
    exit 1
fi

# Check architecture
ARCH=$(uname -m)
if [ "$ARCH" != "x86_64" ] && [ "$ARCH" != "amd64" ]; then
    echo "Error: Only x86_64 is supported. Detected: $ARCH"
    exit 1
fi

echo "Detected: linux/x86_64"

TARBALL="dumber_linux_x86_64.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${TARBALL}"

# Create temporary directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download checksums file
CHECKSUMS_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"
echo "Downloading checksums..."
if ! curl -fsSL "$CHECKSUMS_URL" -o "$TMP_DIR/checksums.txt"; then
    echo "Error: Failed to download checksums file"
    exit 1
fi

echo "Downloading ${TARBALL}..."
if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$TARBALL"; then
    echo "Error: Failed to download ${TARBALL}"
    exit 1
fi

# Verify checksum
echo "Verifying checksum..."
EXPECTED_CHECKSUM=$(grep -E "[[:space:]]${TARBALL}\$" "$TMP_DIR/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED_CHECKSUM" ]; then
    echo "Error: Could not find checksum for ${TARBALL}"
    exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_CHECKSUM=$(sha256sum "$TMP_DIR/$TARBALL" | awk '{print $1}')
else
    echo "Error: sha256sum not found"
    exit 1
fi

if [ "$EXPECTED_CHECKSUM" != "$ACTUAL_CHECKSUM" ]; then
    echo "Error: Checksum verification failed!"
    echo "Expected: $EXPECTED_CHECKSUM"
    echo "Actual:   $ACTUAL_CHECKSUM"
    exit 1
fi
echo "Checksum verified."

echo "Extracting..."
tar -xzf "$TMP_DIR/$TARBALL" -C "$TMP_DIR"

# Find the dumber binary (handles versioned subdirectory)
BINARY=$(find "$TMP_DIR" -name "dumber" -type f -executable | head -1)
if [ -z "$BINARY" ]; then
    echo "Error: Could not find dumber binary in archive"
    exit 1
fi

echo "Installing to ${INSTALL_DIR}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "$BINARY" "$INSTALL_DIR/dumber"
else
    echo "sudo required to install to ${INSTALL_DIR}"
    sudo mv "$BINARY" "$INSTALL_DIR/dumber"
fi

# Verify installation
if command -v dumber >/dev/null 2>&1; then
    echo ""
    echo "Dumber installed successfully!"
    dumber about
else
    echo ""
    echo "Installation complete. Add ${INSTALL_DIR} to your PATH if needed."
fi
