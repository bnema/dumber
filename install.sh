#!/bin/sh
set -e

# Dumber installer script
# Usage: curl -fsSL https://dumber.bnema.dev/install | sh
# Usage with pre-release: curl -fsSL https://dumber.bnema.dev/install | DUMBER_PRERELEASE=1 sh

REPO="bnema/dumber"
VERSION="${DUMBER_VERSION:-latest}"

# Prefer ~/.local/bin if it exists and is in PATH, otherwise /usr/local/bin
if [ -d "$HOME/.local/bin" ]; then
    INSTALL_DIR="$HOME/.local/bin"
    case ":$PATH:" in
        *":$HOME/.local/bin:"*) ;;
        *) echo "Warning: $HOME/.local/bin is not in your PATH" ;;
    esac
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

# Determine version to download
if [ "$VERSION" = "latest" ] && [ -n "$DUMBER_PRERELEASE" ]; then
    echo "Finding latest pre-release..."
    RELEASE_DATA=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=100" 2>/dev/null || echo "")
    if [ -z "$RELEASE_DATA" ]; then
        echo "Error: Failed to fetch release information from GitHub API"
        exit 1
    fi
    # Prefer jq for reliable JSON parsing of RELEASE_DATA into VERSION.
    # The awk branch is a portability fallback that assumes prerelease and
    # tag_name appear in the same '{' fragment; JSON strings containing '{'
    # or API shape changes can break this extraction.
    if command -v jq >/dev/null 2>&1; then
        VERSION=$(echo "$RELEASE_DATA" | jq -r '.[] | select(.prerelease == true) | .tag_name' | head -1)
    else
        VERSION=$(echo "$RELEASE_DATA" | awk '
            BEGIN { RS = "{" }
            /"prerelease"[[:space:]]*:[[:space:]]*true/ {
                tag = ""
                if (match($0, /"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"/)) {
                    tag = substr($0, RSTART, RLENGTH)
                    sub(/.*"tag_name"[[:space:]]*:[[:space:]]*"/, "", tag)
                    sub(/".*/, "", tag)
                }
                if (tag != "") {
                    print tag
                    exit
                }
            }
        ')
    fi
    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest pre-release version"
        exit 1
    fi
    echo "Using pre-release version: ${VERSION}"
elif [ "$VERSION" = "latest" ]; then
    echo "Using latest stable release"
    RELEASE_DATA=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || echo "")
    if [ -z "$RELEASE_DATA" ]; then
        echo "Error: Failed to fetch latest release information from GitHub API"
        exit 1
    fi
    if command -v jq >/dev/null 2>&1; then
        VERSION=$(echo "$RELEASE_DATA" | jq -r '.tag_name')
    else
        VERSION=$(echo "$RELEASE_DATA" | awk '
            /"tag_name"[[:space:]]*:/ {
                tag = $0
                sub(/.*"tag_name"[[:space:]]*:[[:space:]]*"/, "", tag)
                sub(/".*/, "", tag)
                print tag
                exit
            }
        ')
    fi
    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest stable version"
        exit 1
    fi
    echo "Resolved version: ${VERSION}"
else
    echo "Using version: ${VERSION}"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

# Create temporary directory
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Download checksums file
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
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
