#!/bin/bash

# 切换到项目根目录
cd "$(dirname "$0")/.."

# 检查可执行文件是否存在
if [ ! -f "./nexus-prover" ]; then
    echo "错误: nexus-prover 可执行文件不存在，请先编译程序"
    echo "运行: ./scripts/build.sh"
    exit 1
fi

# 显示帮助信息
if [ "$1" = "-h" ] || [ "$1" = "--help" ] || [ "$1" = "help" ]; then
    echo "🚀 Nexus Prover CLI 运行脚本"
    echo ""
    echo "用法:"
    echo "  ./scripts/run.sh [选项]"
    echo ""
    echo "选项:"
    echo "  test                   运行测试模式"
    echo "  -h, --help, help       显示此帮助信息"
    echo "  -v, --version, version 显示版本信息"
    echo ""
    echo "示例:"
    echo "  ./scripts/run.sh                    # 使用默认配置文件 configs/config.json"
    echo "  ./scripts/run.sh -c my_config.json  # 使用指定配置文件"
    echo "  ./scripts/run.sh test               # 运行测试模式"
    echo "  ./scripts/run.sh -h                 # 显示帮助信息"
    exit 0
fi

# 运行程序
echo "🚀 启动程序..."
./nexus-prover "$@" 