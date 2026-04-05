# 远程桌面控制软件

这是一个基于Go语言开发的远程桌面控制软件，分为服务器端、被控端和控制端三部分，支持P2P连接和中继 fallback 机制。

## 项目结构

```
remote-desktop/
├── server/              # 服务器端
│   ├── main.go          # 服务器主入口
│   ├── relay.go         # 中继服务器实现
│   └── config.yaml      # 服务器配置文件
├── client/              # 客户端
│   ├── agent/           # 被控端
│   │   ├── main.go      # 被控端主入口
│   │   └── config.yaml  # 被控端配置文件
│   └── controller/      # 控制端
│       ├── main.go      # 控制端主入口
│       └── config.yaml  # 控制端配置文件
├── common/              # 通用代码
│   ├── config/          # 配置加载
│   │   └── config.go    # 配置加载实现
│   └── network/         # 网络工具
│       └── p2p.go       # P2P连接实现
├── scripts/             # 启动脚本
│   ├── start-server.sh  # 启动服务器
│   ├── start-agent.sh   # 启动被控端
│   └── start-controller.sh  # 启动控制端
├── go.mod               # Go模块文件
└── README.md            # 项目说明
```

## 核心功能

1. **P2P连接**：优先使用P2P连接，提供低延迟的远程控制体验
2. **中继 fallback**：当P2P连接失败时，自动切换到中继模式
3. **跨平台支持**：被控端和控制端支持Windows和Linux
4. **屏幕捕获**：支持不同平台的屏幕捕获方法
5. **远程控制**：支持鼠标、键盘和剪贴板操作
6. **安全认证**：提供身份认证和数据加密

## 技术栈

- **Go 1.20**：主要开发语言
- **WebRTC**：用于P2P连接
- **Gin**：用于服务器端API
- **WebSocket**：用于服务器与客户端通信
- **YAML**：用于配置文件

## 快速开始

### 1. 配置服务器

编辑 `server/config.yaml` 文件，设置服务器的监听地址、端口和其他配置。

### 2. 配置被控端

编辑 `client/agent/config.yaml` 文件，设置服务器地址和设备信息。

### 3. 配置控制端

编辑 `client/controller/config.yaml` 文件，设置服务器地址和显示配置。

### 4. 启动服务器

```bash
./scripts/start-server.sh
```

### 5. 启动被控端

```bash
./scripts/start-agent.sh
```

### 6. 启动控制端

```bash
./scripts/start-controller.sh
```

## 安全注意事项

1. 请确保使用强密码和安全的认证机制
2. 建议在防火墙中限制服务器的访问范围
3. 定期更新软件以修复安全漏洞
4. 避免在不安全的网络环境中使用

## 开发指南

### 依赖管理

使用Go Modules进行依赖管理：

```bash
go mod tidy
```

### 构建

```bash
# 构建服务器
go build -o server/server ./server

# 构建被控端
go build -o client/agent/agent ./client/agent

# 构建控制端
go build -o client/controller/controller ./client/controller
```

### 测试

```bash
# 运行测试
go test ./...
```

## 部署建议

1. **服务器端**：建议部署在云服务器上，确保有固定的公网IP
2. **被控端**：在需要远程控制的设备上安装并运行
3. **控制端**：在控制设备上安装并运行

## 故障排查

1. **连接失败**：检查网络连接、防火墙设置和配置文件
2. **P2P连接失败**：检查NAT类型和网络环境，可能需要使用中继模式
3. **屏幕捕获失败**：检查权限设置和屏幕捕获方法
4. **远程控制无响应**：检查网络延迟和连接状态

## 许可证

MIT License
