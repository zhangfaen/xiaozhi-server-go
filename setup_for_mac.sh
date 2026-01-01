#!/bin/bash
# =============================================================================
# 小智服务器 - Mac 环境安装脚本
# =============================================================================
# 功能:
#   1. 检测芯片架构 (Apple Silicon vs Intel)
#   2. 检测并安装必要的系统依赖 (Homebrew, opus, pkg-config)
#   3. 检测 Go 版本并提示升级
#   4. 检测/安装 Ollama (可选,用于本地LLM)
#   5. 配置 Go 环境变量
#   6. 创建必要目录
#   7. 下载 Go 依赖并编译测试
# =============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目目录
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  小智服务器 - Mac 环境安装脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# -----------------------------------------------------------------------------
# 0. 检测芯片架构
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[0/7] 检测芯片架构...${NC}"

# 检测是否为 Apple Silicon
# /opt/homebrew 只存在于 Apple Silicon Mac
if [ -d "/opt/homebrew" ]; then
    IS_ARM64=true
    echo -e "${GREEN}  检测到 Apple Silicon (M1/M2/M3)${NC}"
    BREW="/opt/homebrew/bin/brew"
    echo -e "${GREEN}  使用 Homebrew: $BREW${NC}"
else
    IS_ARM64=false
    echo -e "${GREEN}  检测到 Intel 芯片${NC}"
    BREW="/usr/local/bin/brew"
fi

# -----------------------------------------------------------------------------
# 1. 检测 Homebrew
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[1/7] 检测 Homebrew...${NC}"
if ! command -v brew &> /dev/null; then
    echo -e "${YELLOW}  Homebrew 未安装,正在安装...${NC}"
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
    if [ $? -ne 0 ]; then
        echo -e "${RED}  Homebrew 安装失败,请手动安装:${NC}"
        echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        exit 1
    fi
    # 重新检测 brew 路径
    if [[ "$(uname -m)" == "arm64" ]] && [ -f "/opt/homebrew/bin/brew" ]; then
        BREW="/opt/homebrew/bin/brew"
    else
        BREW="/usr/local/bin/brew"
    fi
    echo -e "${GREEN}  Homebrew 安装成功${NC}"
else
    echo -e "${GREEN}  Homebrew 已安装${NC}"
fi

# -----------------------------------------------------------------------------
# 2. 检测/安装系统依赖
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[2/7] 检测系统依赖 (opus, pkg-config)...${NC}"

# Apple Silicon 需要使用 arch -arm64 确保原生编译
if [ "$IS_ARM64" = true ]; then
    BREW_CMD="arch -arm64 $BREW"
else
    BREW_CMD="$BREW"
fi

# 安装/更新 opus
if ! $BREW_CMD list opus &> /dev/null; then
    echo -e "${YELLOW}  安装 opus...${NC}"
    $BREW_CMD install opus
    echo -e "${GREEN}  opus 安装成功${NC}"
else
    echo -e "${GREEN}  opus 已安装${NC}"
fi

# 安装/更新 pkg-config
if ! $BREW_CMD list pkg-config &> /dev/null; then
    echo -e "${YELLOW}  安装 pkg-config...${NC}"
    $BREW_CMD install pkg-config
    echo -e "${GREEN}  pkg-config 安装成功${NC}"
else
    echo -e "${GREEN}  pkg-config 已安装${NC}"
fi

# 设置 PKG_CONFIG_PATH
if [ "$IS_ARM64" = true ]; then
    export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig:$PKG_CONFIG_PATH"
else
    export PKG_CONFIG_PATH="/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH"
fi

# 验证 opus
if ! pkg-config --exists opus; then
    echo -e "${RED}  错误: opus 库未正确安装,尝试修复...${NC}"
    $BREW_CMD link --force opus 2>/dev/null || true
    export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig:/usr/local/lib/pkgconfig:$PKG_CONFIG_PATH"
    if ! pkg-config --exists opus; then
        echo -e "${RED}  opus 验证失败${NC}"
        echo "  尝试手动运行: $BREW_CMD link --force opus"
        exit 1
    fi
fi
echo -e "${GREEN}  opus 验证通过${NC}"

# -----------------------------------------------------------------------------
# 3. 检测 Go 版本
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[3/7] 检测 Go 环境...${NC}"

if ! command -v go &> /dev/null; then
    echo -e "${RED}  Go 未安装,正在安装...${NC}"
    $BREW_CMD install go
fi

GO_VERSION=$(go version 2>/dev/null | sed 's/.*go\([0-9.]*\).*/\1/' || echo "0")
GO_MAJOR=$(echo $GO_VERSION | cut -d. -f1)
GO_MINOR=$(echo $GO_VERSION | cut -d. -f2)

echo -e "${GREEN}  当前 Go 版本: $GO_VERSION${NC}"

# 需要 Go 1.24+
REQUIRED_MAJOR=1
REQUIRED_MINOR=24

if [ "$GO_MAJOR" -lt "$REQUIRED_MAJOR" ] || ([ "$GO_MAJOR" -eq "$REQUIRED_MAJOR" ] && [ "$GO_MINOR" -lt "$REQUIRED_MINOR" ]); then
    echo -e "${YELLOW}  警告: 当前 Go 版本 ($GO_VERSION) 小于要求的 1.24${NC}"
    echo -e "${YELLOW}  正在尝试通过 Homebrew 升级 Go...${NC}"

    # 尝试升级
    $BREW_CMD upgrade go 2>/dev/null || $BREW_CMD install go

    # 重新检查版本
    GO_VERSION=$(go version 2>/dev/null | sed 's/.*go\([0-9.]*\).*/\1/' || echo "0")
    GO_MAJOR=$(echo $GO_VERSION | cut -d. -f1)
    GO_MINOR=$(echo $GO_VERSION | cut -d. -f2)

    if [ "$GO_MAJOR" -lt "$REQUIRED_MAJOR" ] || ([ "$GO_MAJOR" -eq "$REQUIRED_MAJOR" ] && [ "$GO_MINOR" -lt "$REQUIRED_MINOR" ]); then
        echo -e "${RED}  Homebrew 安装的 Go 版本仍低于 1.24${NC}"
        echo -e "${YELLOW}  请手动下载安装最新 Go:${NC}"
        echo "  1. 下载: https://go.dev/dl/go1.24.0.darwin-arm64.pkg (Apple Silicon)"
        echo "     或: https://go.dev/dl/go1.24.0.darwin-amd64.pkg (Intel)"
        echo "  2. 安装后运行: source ~/.zshrc 或重启终端"
        echo ""
        read -p "  是否继续安装? (可能编译失败) [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "  安装已取消"
            exit 0
        fi
    else
        echo -e "${GREEN}  Go 升级成功: $GO_VERSION${NC}"
    fi
else
    echo -e "${GREEN}  Go 版本满足要求 (>= 1.24)${NC}"
fi

# -----------------------------------------------------------------------------
# 4. 设置环境变量
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[4/7] 配置环境变量...${NC}"

# 设置 PKG_CONFIG_PATH (根据架构选择正确的路径)
if [ "$IS_ARM64" = true ]; then
    PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig"
else
    PKG_CONFIG_PATH="/usr/local/lib/pkgconfig"
fi
export PKG_CONFIG_PATH="$PKG_CONFIG_PATH"
echo -e "${GREEN}  PKG_CONFIG_PATH=$PKG_CONFIG_PATH${NC}"

# 启用 CGO (编译 opus 需要)
export CGO_ENABLED=1
echo -e "${GREEN}  CGO_ENABLED=1${NC}"

# Go proxy 设置 (国内加速)
if [ -z "$(go env GOPROXY)" ] || [[ "$(go env GOPROXY)" == *"proxy.golang.org"* ]]; then
    echo -e "${YELLOW}  设置 Go proxy 为国内镜像...${NC}"
    go env -w GOPROXY=https://goproxy.cn,direct
fi
echo -e "${GREEN}  GOPROXY=$(go env GOPROXY)${NC}"

# 保存到 .env
cat > .env << EOF
# 小智服务器 - Mac 环境变量配置
# 此文件由 setup_for_mac.sh 自动生成
# 生成时间: $(date)

# CGO 编译开关 (必须启用)
export CGO_ENABLED=1

# opus 库路径
export PKG_CONFIG_PATH="$PKG_CONFIG_PATH"

# Go 模块代理 (国内加速)
export GOPROXY=https://goproxy.cn,direct
EOF

echo -e "${GREEN}  环境变量已保存到 .env${NC}"

# -----------------------------------------------------------------------------
# 5. 创建必要目录
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[5/7] 创建必要目录...${NC}"

mkdir -p tmp
mkdir -p logs
mkdir -p music

echo -e "${GREEN}  tmp/    - TTS音频临时目录${NC}"
echo -e "${GREEN}  logs/   - 日志目录${NC}"
echo -e "${GREEN}  music/  - 音乐播放目录${NC}"

# -----------------------------------------------------------------------------
# 6. 下载 Go 依赖
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[6/7] 下载 Go 依赖...${NC}"

echo -e "${YELLOW}  执行: go mod download${NC}"
if go mod download; then
    echo -e "${GREEN}  依赖下载成功${NC}"
else
    echo -e "${RED}  依赖下载失败,尝试清理重试...${NC}"
    go mod tidy
    go mod download
fi

# 编译测试
echo ""
echo -e "${YELLOW}  编译测试...${NC}"
if go build -o /dev/null ./src/main.go 2>/dev/null; then
    echo -e "${GREEN}  编译测试通过!${NC}"
else
    echo -e "${RED}  编译测试失败,请检查错误信息${NC}"
    echo -e "${YELLOW}  尝试查看详细错误:${NC}"
    go build -v ./src/main.go
fi

# -----------------------------------------------------------------------------
# 7. Ollama 检测 (可选)
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[7/7] Ollama 检测 (可选)...${NC}"

if command -v ollama &> /dev/null; then
    echo -e "${GREEN}  Ollama 已安装${NC}"
    echo -e "${GREEN}  当前模型列表:${NC}"
    ollama list 2>/dev/null || echo "    (无法获取模型列表,可能服务未启动)"
    echo ""
    echo -e "${BLUE}  提示: 如果要使用本地 LLM,请确保 Ollama 服务正在运行:${NC}"
    echo "    brew services start ollama  # 启动服务"
    echo "    ollama pull qwen3            # 下载模型"
else
    echo -e "${YELLOW}  Ollama 未安装 (可选,用于本地 LLM)${NC}"
    echo -e "${BLUE}  安装命令: brew install ollama${NC}"
fi

# -----------------------------------------------------------------------------
# 完成
# -----------------------------------------------------------------------------
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  环境安装完成!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}下一步:${NC}"
echo "  1. 配置 config.yaml (参考 config.example.yaml)"
echo "  2. 运行 ./run_on_mac.sh 启动服务"
echo ""
echo -e "${BLUE}服务端口:${NC}"
echo "  - WebSocket: 8000"
echo "  - HTTP API:  8080"
echo "  - Swagger:   http://localhost:8080/swagger/index.html"
echo ""
