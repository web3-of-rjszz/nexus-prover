#!/bin/bash

set -e

echo "🔨 静态编译 Nexus Prover CLI..."

# 切换到项目根目录
cd "$(dirname "$0")/.."

# 设置环境变量抑制告警
export CGO_ENABLED=1
export CGO_CFLAGS="-w"
export CGO_LDFLAGS="-w"

# 编译命令，添加参数抑制告警
go build -ldflags="-extldflags=-static -s -w" -o nexus-prover ./cmd/nexus-prover 2>/dev/null

if [ $? -eq 0 ]; then
    echo "✅ 静态编译完成！"
    ldd nexus-prover 2>/dev/null || echo "静态链接的可执行文件"
    echo ""
    echo "🚀 使用方法:"
    echo "  ./nexus-prover"
    echo "  ./nexus-prover --process-isolation"
    echo "  ./nexus-prover -c configs/config.json"
else
    echo "❌ 静态编译失败"
    exit 1
fi 