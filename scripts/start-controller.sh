#!/bin/bash

# 启动控制端
cd "$(dirname "$0")/../client/controller"

# 检查配置文件
if [ ! -f "config.yaml" ]; then
    echo "Error: config.yaml not found!"
    echo "Please copy config.yaml.example to config.yaml and modify it."
    exit 1
fi

# 启动控制端
echo "Starting controller..."
go run main.go
