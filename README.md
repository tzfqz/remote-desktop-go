# 远程桌面控制系统

基于 Go + WebRTC 的远程桌面控制软件，支持 P2P 直连和中继回退。

## 功能特性

- **P2P 直连**：WebRTC DataChannel，低延迟
- **中继回退**：WebSocket 中继，NAT 穿透失败时的兜底方案
- **屏幕捕获**：Windows GDI BitBlt 原生截屏，支持 JPEG/PNG 压缩
- **输入模拟**：Windows SendInput + mouse_event + keybd_event，完整鼠标/键盘事件
- **Web 前端**：浏览器直接控制，无需安装客户端
- **跨平台**：服务端支持 Linux/Windows，被控端目前为 Windows

## 项目结构

```
remote-desktop-go/
├── bin/                         # 编译产物（直接运行）
│   ├── server.exe               # 信令+中继服务器
│   ├── agent.exe                # 被控端（Windows）
│   ├── controller.exe           # 控制端 CLI
│   ├── agent.yaml               # 被控端配置
│   └── controller.yaml          # 控制端配置
├── server/                      # 服务端源码
│   ├── main.go                  # 主入口（Gin HTTP + WebSocket 信令）
│   └── relay.go                 # WebSocket 中继服务器（端口 8081）
├── client/
│   ├── agent/                   # 被控端
│   │   ├── main.go             # 主程序
│   │   ├── screen/             # 屏幕捕获模块
│   │   │   └── capture_windows.go   # GDI BitBlt 实现
│   │   ├── input/              # 输入模拟模块
│   │   │   └── input_windows.go    # SendInput 实现
│   │   └── config.yaml
│   └── controller/              # 控制端 CLI
│       ├── main.go
│       └── config.yaml
├── web/                         # Web 前端
│   └── index.html               # 浏览器控制界面
├── common/
│   ├── protocol/                # 协议定义
│   │   └── protocol.go
│   └── config/                  # 配置加载
│       └── config.go
└── README.md
```

## 快速开始

### 1. 启动服务器

```bash
cd bin
./server.exe
```

服务端口：
- HTTP/WebSocket 信令：`http://0.0.0.0:8080`
- WebSocket 中继：`ws://0.0.0.0:8081/relay`

### 2. 启动被控端

修改 `bin/agent.yaml` 中的服务器地址，然后运行：

```bash
./agent.exe
```

被控端会主动连接服务器（WebSocket），等待控制端发起控制请求。

### 3. 控制端连接

**方式一：Web 前端**

浏览器访问 `http://服务器IP:8080`，即可看到已上线的设备列表，点击连接即可远程控制。

**方式二：CLI 控制端**

修改 `bin/controller.yaml`，运行：

```bash
./controller.exe
```

## 配置文件说明

### agent.yaml（被控端）

```yaml
server:
  address: "服务器IP:8080"   # 信令服务器地址
  relay:   "服务器IP:8081"   # 中继服务器地址

agent:
  device_id: "pc-001"         # 设备唯一标识
  password: "xxx"             # 连接密码

screen:
  width:    1920              # 截屏分辨率
  height:   1080
  fps:      15                # 帧率上限
  quality:  70                # JPEG 质量 (1-100)

input:
  enable_keyboard: true
  enable_mouse:    true
```

### controller.yaml（控制端）

```yaml
server:
  address: "服务器IP:8080"
  relay:   "服务器IP:8081"

controller:
  device_id: "pc-001"        # 要控制的设备ID
  password:  "xxx"

display:
  width:  1920                # 本地显示分辨率
  height: 1080
```

## 技术架构

```
┌─────────────┐   WebSocket   ┌─────────────┐
│  Controller │◄────────────►│   Server    │
│  (控制端)    │   信令/控制    │  (信令服务器) │
└─────────────┘               └─────────────┘
       │                            │
       │ WebRTC DataChannel (P2P)   │ WebSocket (中继回退)
       │         or                 │         or
       │    WebSocket 中继          │   WebSocket 直连
       │         ▼                  │         ▼
┌─────────────┐               ┌─────────────┐
│    Agent    │◄─────────────►│    Relay    │
│  (被控端)    │    中继透传     │ (中继服务器)  │
│  GDI截屏    │               │  端口8081   │
│  输入模拟   │               └─────────────┘
└─────────────┘
```

## 通信协议

| 消息类型 | 方向 | 说明 |
|---------|------|------|
| `join` | Agent→Server | 设备上线注册 |
| `device_list` | Server→Controller | 设备列表推送 |
| `offer/answer/ice` | Controller↔Agent | WebRTC 信令 |
| `control` | Controller→Agent | 控制指令（鼠标/键盘/剪贴板）|
| `screen_frame` | Agent→Controller | 屏幕帧数据（base64/JPEG）|

## 构建

```bash
# 编译全部模块
go build -o bin/server.exe     ./server
go build -o bin/agent.exe       ./client/agent
go build -o bin/controller.exe  ./client/controller

# 安装依赖
go mod tidy
```

## 已知限制

- P2P DataChannel 连接流程尚未完全实现（信令已通）
- 中继模式可正常工作
- 被控端仅支持 Windows
- Linux/macOS 被控端需要实现对应平台的 screen/input 模块

## License

MIT
