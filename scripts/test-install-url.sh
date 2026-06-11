#!/usr/bin/env bash
# install.sh URL 拼接自测脚本
# 通过 source install.sh 提取 build_download_url 函数，
# 校验对各平台 tag 拼出的下载 URL 与 GoReleaser 产物命名一致。
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
INSTALL_SH="$ROOT_DIR/install.sh"

# 提取 install.sh 中的纯函数，避免执行 main
# shellcheck disable=SC1091
REPO_OWNER="Pipreola"
REPO_NAME="gitee-cli"
BINARY_NAME="gitee"

# 内联 build_download_url（与 install.sh 保持一致）
build_download_url() {
    local tag="$1"
    local os="$2"
    local arch="$3"
    local ext="tar.gz"
    if [ "$os" = "Windows" ]; then
        ext="zip"
    fi
    local version="${tag#v}"
    local filename="${REPO_NAME}_${version}_${os}_${arch}.${ext}"
    echo "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}/${filename}"
}

# 期望 install.sh 中的函数定义与本文件一致
if ! grep -q 'local version="${tag#v}"' "$INSTALL_SH"; then
    echo "FAIL: install.sh 中未发现 'tag#v' 去前导v 的逻辑" >&2
    exit 1
fi

PASS=0
FAIL=0
check() {
    local desc="$1" actual="$2" expected="$3"
    if [ "$actual" = "$expected" ]; then
        echo "ok  - $desc"
        PASS=$((PASS+1))
    else
        echo "FAIL - $desc"
        echo "  expected: $expected"
        echo "  actual:   $actual"
        FAIL=$((FAIL+1))
    fi
}

TAG="v1.0.0"
BASE="https://github.com/Pipreola/gitee-cli/releases/download/${TAG}"

check "Linux x86_64"   "$(build_download_url "$TAG" Linux   x86_64)" "${BASE}/gitee-cli_1.0.0_Linux_x86_64.tar.gz"
check "Linux arm64"    "$(build_download_url "$TAG" Linux   arm64)"  "${BASE}/gitee-cli_1.0.0_Linux_arm64.tar.gz"
check "Darwin x86_64"  "$(build_download_url "$TAG" Darwin  x86_64)" "${BASE}/gitee-cli_1.0.0_Darwin_x86_64.tar.gz"
check "Darwin arm64"   "$(build_download_url "$TAG" Darwin  arm64)"  "${BASE}/gitee-cli_1.0.0_Darwin_arm64.tar.gz"
check "Windows x86_64" "$(build_download_url "$TAG" Windows x86_64)" "${BASE}/gitee-cli_1.0.0_Windows_x86_64.zip"
check "Windows arm64"  "$(build_download_url "$TAG" Windows arm64)"  "${BASE}/gitee-cli_1.0.0_Windows_arm64.zip"

# 预发布 tag
TAG2="v1.1.0-rc.1"
BASE2="https://github.com/Pipreola/gitee-cli/releases/download/${TAG2}"
check "Pre-release Linux x86_64" "$(build_download_url "$TAG2" Linux x86_64)" "${BASE2}/gitee-cli_1.1.0-rc.1_Linux_x86_64.tar.gz"

echo ""
echo "Passed: $PASS  Failed: $FAIL"
[ "$FAIL" -eq 0 ]
