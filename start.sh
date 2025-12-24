#!/bin/bash

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 项目信息
PROJECT_NAME="anti2api"
BINARY_NAME="anti2api"
BUILD_DIR="."
CMD_DIR="./cmd/server"

echo -e "${BLUE}=====================================${NC}"
echo -e "${BLUE}  Anti2API Golang Server Launcher${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo -e "${RED}✗ Error: Go is not installed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Go version: $(go version)${NC}"

# 检查是否需要重新编译
NEED_BUILD=false

if [ ! -f "${BINARY_NAME}" ]; then
    echo -e "${YELLOW}⚠ Binary not found, building...${NC}"
    NEED_BUILD=true
else
    # 检查源文件是否有更新
    BINARY_TIME=$(stat -c %Y "${BINARY_NAME}" 2>/dev/null || stat -f %m "${BINARY_NAME}" 2>/dev/null)

    # 查找所有 .go 文件，检查是否有比二进制文件更新的
    NEWER_FILES=$(find . -name "*.go" -newer "${BINARY_NAME}" 2>/dev/null | wc -l)

    if [ "$NEWER_FILES" -gt 0 ]; then
        echo -e "${YELLOW}⚠ Source files changed, rebuilding...${NC}"
        NEED_BUILD=true
    else
        echo -e "${GREEN}✓ Binary is up to date${NC}"
    fi
fi

# 编译项目
if [ "$NEED_BUILD" = true ]; then
    echo -e "${BLUE}→ Building ${PROJECT_NAME}...${NC}"

    # 清理旧的二进制文件
    if [ -f "${BINARY_NAME}" ]; then
        rm -f "${BINARY_NAME}"
    fi

    # 编译
    if go build -ldflags="-s -w" -o "${BINARY_NAME}" "${CMD_DIR}"; then
        echo -e "${GREEN}✓ Build successful${NC}"

        # 显示二进制文件大小
        SIZE=$(du -h "${BINARY_NAME}" | cut -f1)
        echo -e "${GREEN}  Binary size: ${SIZE}${NC}"
    else
        echo -e "${RED}✗ Build failed${NC}"
        exit 1
    fi
fi

# 检查 .env 文件
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}⚠ Warning: .env file not found${NC}"
    echo -e "${YELLOW}  Using .env.example as reference${NC}"
    if [ -f ".env.example" ]; then
        echo -e "${YELLOW}  You can copy it: cp .env.example .env${NC}"
    fi
fi

# 启动服务器
echo ""
echo -e "${BLUE}=====================================${NC}"
echo -e "${GREEN}→ Starting ${PROJECT_NAME} server...${NC}"
echo -e "${BLUE}=====================================${NC}"
echo ""

# 执行二进制文件，传递所有参数
exec "./${BINARY_NAME}" "$@"
