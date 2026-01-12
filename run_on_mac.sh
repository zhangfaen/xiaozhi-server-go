#!/bin/bash
# =============================================================================
# 小智服务器 - Mac 启动脚本
# =============================================================================
# 功能:
#   1. 加载环境变量
#   2. 检查配置文件
#   3. 编译项目
#   4. 启动服务
# =============================================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 项目目录
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

# PID 文件
PID_FILE="$PROJECT_DIR/xiaozhi-server.pid"
LOG_FILE="$PROJECT_DIR/logs/server.log"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}  小智服务器 - 启动脚本${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# -----------------------------------------------------------------------------
# 1. 加载环境变量
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[1/5] 加载环境变量...${NC}"

if [ -f .env ]; then
    set -a  # 自动导出
    source .env
    set +a
    echo -e "${GREEN}  已加载 .env${NC}"
else
    echo -e "${YELLOW}  未找到 .env,使用默认设置${NC}"
fi

# 确保 CGO 启用
export CGO_ENABLED=1

# 确保 PKG_CONFIG_PATH (根据架构: /opt/homebrew 仅 Apple Silicon)
if [ -z "$PKG_CONFIG_PATH" ]; then
    if [ -d "/opt/homebrew" ]; then
        export PKG_CONFIG_PATH="/opt/homebrew/lib/pkgconfig"
    else
        export PKG_CONFIG_PATH="/usr/local/lib/pkgconfig"
    fi
fi

echo -e "${GREEN}  CGO_ENABLED=$CGO_ENABLED${NC}"
echo -e "${GREEN}  PKG_CONFIG_PATH=$PKG_CONFIG_PATH${NC}"

# -----------------------------------------------------------------------------
# 2. 检查配置文件
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[2/5] 检查配置文件...${NC}"

if [ ! -f config.yaml ]; then
    echo -e "${RED}  错误: 未找到 config.yaml${NC}"
    echo "  请复制 config.yaml:"
    echo "    cp config.yaml.example config.yaml"
    exit 1
fi
echo -e "${GREEN}  config.yaml 存在${NC}"

# 删除数据库，确保每次从 config.yaml 加载最新配置
if [ -f config.db ]; then
    echo -e "${YELLOW}  删除旧数据库 config.db (确保从 config.yaml 加载)${NC}"
    rm -f config.db
fi

# -----------------------------------------------------------------------------
# 3. 检查端口占用
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[3/5] 检查端口占用...${NC}"

check_port() {
    local port=$1
    if lsof -i :$port &> /dev/null; then
        echo -e "${RED}  端口 $port 已被占用:${NC}"
        lsof -i :$port | head -5
        return 1
    fi
    return 0
}

WS_PORT=$(grep -A2 "websocket:" config.yaml | grep "port:" | grep -oP '\d+' || echo "8000")
HTTP_PORT=$(grep -A2 "web:" config.yaml | grep "port:" | grep -oP '\d+' | head -1 || echo "8080")

echo -e "${CYAN}  WebSocket 端口: $WS_PORT${NC}"
echo -e "${CYAN}  HTTP 端口:     $HTTP_PORT${NC}"

if ! check_port $WS_PORT; then
    echo -e "${RED}  错误: WebSocket 端口被占用${NC}"
    exit 1
fi

if ! check_port $HTTP_PORT; then
    echo -e "${RED}  错误: HTTP 端口被占用${NC}"
    exit 1
fi

echo -e "${GREEN}  端口检查通过${NC}"

# -----------------------------------------------------------------------------
# 4. 编译项目
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[4/5] 编译项目...${NC}"

# 清理旧二进制
if [ -f xiaozhi-server ]; then
    rm -f xiaozhi-server
fi

echo -e "${YELLOW}  执行: go build -o xiaozhi-server ./src/main.go${NC}"

if go build -o xiaozhi-server ./src/main.go; then
    echo -e "${GREEN}  编译成功!${NC}"
else
    echo -e "${RED}  编译失败${NC}"
    exit 1
fi

# -----------------------------------------------------------------------------
# 5. 启动服务
# -----------------------------------------------------------------------------
echo ""
echo -e "${YELLOW}[5/5] 启动服务...${NC}"

# 确保 logs 目录存在
mkdir -p logs

# 获取本机 IP
if [[ "$OSTYPE" == "darwin"* ]]; then
    LOCAL_IP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || echo "localhost")
else
    LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")
fi

echo -e "${GREEN}  本机 IP: $LOCAL_IP${NC}"
echo ""

# 启动服务
exec ./xiaozhi-server \
    2>&1 | tee -a "$LOG_FILE"
