#!/usr/bin/env bash
# 本地构建 ANL Gateway 镜像的快速脚本，避免在命令行反复输入构建参数。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION_FILE="${REPO_ROOT}/backend/cmd/server/VERSION"
VERSION="${ANL_IMAGE_TAG:-$(tr -d '\r\n' < "${VERSION_FILE}")}"
IMAGE_REF="${ANL_IMAGE_REF:-anlapi:${VERSION}}"
COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || printf 'local')"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

docker build -t "${IMAGE_REF}" \
    --build-arg GOPROXY=https://goproxy.cn,direct \
    --build-arg GOSUMDB=sum.golang.google.cn \
    --build-arg VERSION="${VERSION}" \
    --build-arg COMMIT="${COMMIT}" \
    --build-arg DATE="${DATE}" \
    -f "${REPO_ROOT}/Dockerfile" \
    "${REPO_ROOT}"

printf 'Built %s (version=%s commit=%s date=%s)\n' "${IMAGE_REF}" "${VERSION}" "${COMMIT}" "${DATE}"
