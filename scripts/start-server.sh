#!/bin/bash

# 启动服务器端
cd "$(dirname "$0")/../server"

# 检查配置文件
if [ ! -f "config.yaml" ]; then
    echo "Error: config.yaml not found!"
    echo "Please copy config.yaml.example to config.yaml and modify it."
    exit 1
fi

# 启动服务器
echo "Starting server..."
go run main.go
