#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# ai-switch release build script
# Usage:
#   ./scripts/release.sh [version]
#
# Examples:
#   ./scripts/release.sh              # uses git tag or "dev"
#   ./scripts/release.sh v0.1.0       # specify version
# ============================================================

PROJECT="ai-switch"
CMD="./cmd/server"
DIST_DIR="dist"

VERSION="${1:-}"

if [ -z "$VERSION" ]; then
    VERSION=$(git describe --tags --exact-match 2>/dev/null || echo "dev")
fi

GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w -X main.version=${VERSION} -X main.gitCommit=${GIT_COMMIT} -X main.buildTime=${BUILD_TIME}"
BUILD_OPTS="-trimpath"

# Cross-compilation targets: OS/ARCH
TARGETS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

echo "==> Building frontend..."
make build-ui

echo "==> Building release binaries (version: ${VERSION})..."
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

for target in "${TARGETS[@]}"; do
    IFS="/" read -r GOOS GOARCH <<< "$target"

    BINARY="${PROJECT}"
    if [ "$GOOS" = "windows" ]; then
        BINARY="${BINARY}.exe"
    fi

    PKG_NAME="${PROJECT}-${VERSION#v}-${GOOS}-${GOARCH}"

    echo "    -> ${PKG_NAME}"
    mkdir -p "${DIST_DIR}/${PKG_NAME}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build ${BUILD_OPTS} -ldflags "${LDFLAGS}" \
        -o "${DIST_DIR}/${PKG_NAME}/${BINARY}" ${CMD}

    cp README.md "${DIST_DIR}/${PKG_NAME}/"
    cp LICENSE "${DIST_DIR}/${PKG_NAME}/"

    if [ "$GOOS" = "windows" ]; then
        (cd "${DIST_DIR}" && zip -r "${PKG_NAME}.zip" "${PKG_NAME}")
    else
        tar -czf "${DIST_DIR}/${PKG_NAME}.tar.gz" -C "${DIST_DIR}" "${PKG_NAME}"
    fi
    rm -rf "${DIST_DIR}/${PKG_NAME}"
done

echo "==> Generating checksums..."
(cd "${DIST_DIR}" && sha256sum *.tar.gz *.zip > checksums-sha256.txt)

echo "==> Done! Artifacts in ${DIST_DIR}/:"
ls -lh "${DIST_DIR}/"
