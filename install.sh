#!/usr/bin/env bash
# gitee-cli 一键安装脚本
# 支持 Linux、macOS、Windows (Git Bash/WSL)
# 用法: curl -fsSL https://raw.githubusercontent.com/Pipreola/gitee-cli/main/install.sh | bash

set -e

# 配置
REPO_OWNER="Pipreola"
REPO_NAME="gitee-cli"
BINARY_NAME="gitee"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[信息]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[成功]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[警告]${NC} $1"
}

log_error() {
    echo -e "${RED}[错误]${NC} $1"
}

# 检测操作系统
detect_os() {
    local os
    case "$(uname -s)" in
        Linux*)     os="Linux";;
        Darwin*)    os="Darwin";;
        MINGW*|MSYS*|CYGWIN*)  os="Windows";;
        *)          os="Unknown";;
    esac
    echo "$os"
}

# 检测CPU架构
detect_arch() {
    local arch
    case "$(uname -m)" in
        x86_64|amd64)   arch="x86_64";;
        aarch64|arm64)  arch="arm64";;
        armv7l)         arch="armv7";;
        i386|i686)      arch="i386";;
        *)              arch="unknown";;
    esac
    echo "$arch"
}

# 获取最新版本号
get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$version" ]; then
        log_error "无法获取最新版本号"
        exit 1
    fi
    echo "$version"
}

# 构建下载URL
# 参数: $1=tag(形如 v1.0.0)  $2=os  $3=arch
# 产物名遵循 GoReleaser 默认 {{ .Version }}（不含前导 v），URL 路径使用 tag。
build_download_url() {
    local tag="$1"
    local os="$2"
    local arch="$3"
    local ext="tar.gz"

    # Windows 使用 zip
    if [ "$os" = "Windows" ]; then
        ext="zip"
    fi

    # 去掉 tag 的前导 'v'，对齐 GoReleaser 产物 {{ .Version }}
    local version="${tag#v}"

    local filename="${REPO_NAME}_${version}_${os}_${arch}.${ext}"
    echo "https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${tag}/${filename}"
}

# 下载并安装
install_binary() {
    local url="$1"
    local os="$2"
    local tmpdir

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    log_info "下载中: $url"

    local archive="${tmpdir}/gitee-cli.tar.gz"
    if [ "$os" = "Windows" ]; then
        archive="${tmpdir}/gitee-cli.zip"
    fi

    if ! curl -fsSL -o "$archive" "$url"; then
        log_error "下载失败"
        exit 1
    fi

    log_info "解压中..."
    cd "$tmpdir"

    if [ "$os" = "Windows" ]; then
        unzip -q "$archive"
    else
        tar -xzf "$archive"
    fi

    # 查找二进制文件
    local binary
    if [ "$os" = "Windows" ]; then
        binary=$(find . -name "${BINARY_NAME}.exe" -type f | head -n 1)
    else
        binary=$(find . -name "${BINARY_NAME}" -type f | head -n 1)
    fi

    if [ -z "$binary" ]; then
        log_error "未找到可执行文件"
        exit 1
    fi

    log_info "安装到 $INSTALL_DIR ..."

    # 检查目标目录是否存在
    if [ ! -d "$INSTALL_DIR" ]; then
        log_warning "目录 $INSTALL_DIR 不存在，尝试创建..."
        if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            log_error "无法创建目录，请使用 sudo 运行或设置 INSTALL_DIR 环境变量"
            exit 1
        fi
    fi

    # 安装二进制文件
    if ! mv "$binary" "$INSTALL_DIR/${BINARY_NAME}" 2>/dev/null; then
        log_warning "需要 sudo 权限安装到 $INSTALL_DIR"
        if ! sudo mv "$binary" "$INSTALL_DIR/${BINARY_NAME}"; then
            log_error "安装失败"
            exit 1
        fi
    fi

    # 设置执行权限
    if [ "$os" != "Windows" ]; then
        chmod +x "$INSTALL_DIR/${BINARY_NAME}" 2>/dev/null || sudo chmod +x "$INSTALL_DIR/${BINARY_NAME}"
    fi

    log_success "gitee-cli 安装成功！"
}

# 验证安装
verify_installation() {
    log_info "验证安装..."

    if command -v "$BINARY_NAME" >/dev/null 2>&1; then
        local version
        version=$("$BINARY_NAME" version 2>&1 || echo "")
        log_success "安装验证成功"
        echo ""
        echo "$version"
        return 0
    else
        log_warning "命令 '$BINARY_NAME' 未在 PATH 中找到"
        log_info "请确保 $INSTALL_DIR 在您的 PATH 环境变量中"
        log_info "您可能需要运行: export PATH=\"\$PATH:$INSTALL_DIR\""
        return 1
    fi
}

# 主函数
main() {
    echo ""
    echo "╔══════════════════════════════════════╗"
    echo "║   gitee-cli 安装脚本                 ║"
    echo "╚══════════════════════════════════════╝"
    echo ""

    # 检查依赖
    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "需要 $cmd 命令，请先安装"
            exit 1
        fi
    done

    # 检测系统信息
    local os arch version url
    os=$(detect_os)
    arch=$(detect_arch)

    log_info "操作系统: $os"
    log_info "架构: $arch"

    if [ "$os" = "Unknown" ] || [ "$arch" = "unknown" ]; then
        log_error "不支持的操作系统或架构"
        exit 1
    fi

    # 获取版本
    log_info "获取最新版本..."
    version=$(get_latest_version)
    log_info "最新版本: $version"

    # 构建下载URL
    url=$(build_download_url "$version" "$os" "$arch")

    # 下载并安装
    install_binary "$url" "$os"

    echo ""

    # 验证安装
    verify_installation

    echo ""
    log_info "开始使用:"
    echo "  1. 登录 Gitee: gitee auth login"
    echo "  2. 查看帮助: gitee --help"
    echo ""
    log_info "文档: https://github.com/${REPO_OWNER}/${REPO_NAME}"
    echo ""
}

# 运行主函数
main "$@"
