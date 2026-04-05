#!/bin/bash

# 启动被控端
cd "$(dirname "$0")/../client/agent"

# 检查配置文件
if [ ! -f "config.yaml" ]; then
    echo "Error: config.yaml not found!"
    echo "Please copy config.yaml.example to config.yaml and modify it."
    exit 1
fi

# 启动被控端
echo "Starting agent..."
go run main.go
