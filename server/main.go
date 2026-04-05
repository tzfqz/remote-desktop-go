package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"remote-desktop/common/config"
	"remote-desktop/common/protocol"
)

// Server 服务器结构
type Server struct {
	router      *gin.Engine
	config      ServerConfig
	relay       *RelayServer
	connections map[string]*ClientConnection
	connMutex   sync.RWMutex
	upgrader    websocket.Upgrader
}

// ClientConnection 客户端连接
type ClientConnection struct {
	ID       string
	Conn     *websocket.Conn
	IsAgent  bool
	SendChan chan []byte
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Server struct {
		Host string `yaml:"host"`
		Port int    `yaml:"port"`
	} `yaml:"server"`
	ICE struct {
		STUNServers  []string `yaml:"stun_servers"`
		TURNServers  []struct {
			URL        string `yaml:"url"`
			Username   string `yaml:"username"`
			Credential string `yaml:"credential"`
		} `yaml:"turn_servers"`
	} `yaml:"ice"`
	Security struct {
		JWTSecret   string `yaml:"jwt_secret"`
		TokenExpiry int    `yaml:"token_expiry"`
	} `yaml:"security"`
	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`
	Relay struct {
		Enable        bool `yaml:"enable"`
		MaxConnections int  `yaml:"max_connections"`
		BufferSize    int  `yaml:"buffer_size"`
	} `yaml:"relay"`
}

// Device 设备信息
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Online   bool   `json:"online"`
}

// main 主函数
func main() {
	// 加载配置
	var config ServerConfig
	err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Load config error: %v", err)
	}

	// 创建服务器
	server := &Server{
		config:      config,
		connections: make(map[string]*ClientConnection),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}

	// 初始化路由器
	server.router = gin.Default()

	// 配置CORS
	server.router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	})

	// 初始化中继服务器
	if config.Relay.Enable {
		server.relay = NewRelayServer(config.Relay.MaxConnections, config.Relay.BufferSize)
		go server.relay.Start()
	}

	// 注册路由
	server.registerRoutes()

	// 启动服务器
	serverAddr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)
	log.Printf("Server starting on %s", serverAddr)
	err = server.router.Run(serverAddr)
	if err != nil {
		log.Fatalf("Server start error: %v", err)
	}
}

// registerRoutes 注册路由
func (s *Server) registerRoutes() {
	// WebSocket连接
	s.router.GET("/ws", s.handleWebSocket)

	// 设备管理
	s.router.GET("/api/devices", s.handleGetDevices)
	s.router.GET("/api/ice-servers", s.handleGetICEServers)

	// 中继服务
	s.router.POST("/api/relay/connect", s.handleRelayConnect)
	s.router.POST("/api/relay/data", s.handleRelayData)

	// 健康检查
	s.router.GET("/health", s.handleHealth)

	// 静态文件服务 - Web控制端
	s.router.Static("/web", "./web")
}

// handleWebSocket 处理WebSocket连接 - 信令交换的核心
func (s *Server) handleWebSocket(c *gin.Context) {
	// 升级HTTP连接到WebSocket
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// 从查询参数获取客户端ID和类型
	clientID := c.Query("id")
	clientType := c.Query("type")
	isAgent := clientType == "agent"

	if clientID == "" {
		conn.Close()
		return
	}

	log.Printf("New connection: %s (type: %s)", clientID, clientType)

	// 创建客户端连接
	clientConn := &ClientConnection{
		ID:       clientID,
		Conn:     conn,
		IsAgent:  isAgent,
		SendChan: make(chan []byte, 100),
	}

	// 存储连接
	s.connMutex.Lock()
	s.connections[clientID] = clientConn
	s.connMutex.Unlock()

	// 启动发送协程
	go s.sendLoop(clientConn)

	// 启动接收协程
	go s.receiveLoop(clientConn)
}

// sendLoop 发送消息循环
func (s *Server) sendLoop(client *ClientConnection) {
	for {
		select {
		case msg := <-client.SendChan:
			err := client.Conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Printf("Send error to %s: %v", client.ID, err)
				s.removeConnection(client.ID)
				return
			}
		}
	}
}

// receiveLoop 接收消息循环 - 信令转发的核心
func (s *Server) receiveLoop(client *ClientConnection) {
	defer func() {
		client.Conn.Close()
		s.removeConnection(client.ID)
	}()

	for {
		// 读取消息
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			log.Printf("Read error from %s: %v", client.ID, err)
			return
		}

		// 解析消息
		var msg protocol.Message
		err = json.Unmarshal(message, &msg)
		if err != nil {
			log.Printf("Parse message error: %v", err)
			continue
		}

		// 设置发送者
		msg.From = client.ID

		// 转发消息到目标
		if msg.To != "" {
			s.forwardMessage(&msg)
		} else {
			// 广播消息或处理特定类型
			s.handleBroadcastMessage(client, &msg)
		}
	}
}

// forwardMessage 转发消息到目标
func (s *Server) forwardMessage(msg *protocol.Message) {
	s.connMutex.RLock()
	targetConn, exists := s.connections[msg.To]
	s.connMutex.RUnlock()

	if !exists {
		log.Printf("Target %s not found", msg.To)
		return
	}

	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Marshal message error: %v", err)
		return
	}

	// 发送消息
	select {
	case targetConn.SendChan <- data:
	default:
		log.Printf("Send channel full for %s", msg.To)
	}
}

// handleBroadcastMessage 处理广播消息
func (s *Server) handleBroadcastMessage(client *ClientConnection, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MsgTypeJoin:
		// 通知设备列表更新
		s.broadcastDeviceList()
	}
}

// broadcastDeviceList 广播设备列表
func (s *Server) broadcastDeviceList() {
	devices := s.getDeviceList()
	msg := protocol.Message{
		Type:    protocol.MsgTypeDeviceList,
		Payload: devices,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	for _, conn := range s.connections {
		select {
		case conn.SendChan <- data:
		default:
		}
	}
}

// getDeviceList 获取设备列表
func (s *Server) getDeviceList() []Device {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()

	devices := make([]Device, 0)
	for _, conn := range s.connections {
		if conn.IsAgent {
			devices = append(devices, Device{
				ID:     conn.ID,
				Name:   conn.ID,
				Online: true,
			})
		}
	}
	return devices
}

// removeConnection 移除连接
func (s *Server) removeConnection(clientID string) {
	s.connMutex.Lock()
	if conn, exists := s.connections[clientID]; exists {
		close(conn.SendChan)
		delete(s.connections, clientID)
	}
	s.connMutex.Unlock()

	// 广播设备列表更新
	s.broadcastDeviceList()
}

// handleGetDevices 获取设备列表
func (s *Server) handleGetDevices(c *gin.Context) {
	devices := s.getDeviceList()
	c.JSON(http.StatusOK, gin.H{
		"devices": devices,
	})
}

// handleGetICEServers 获取ICE服务器配置
func (s *Server) handleGetICEServers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"stun_servers": s.config.ICE.STUNServers,
		"turn_servers": s.config.ICE.TURNServers,
	})
}

// handleRegisterDevice 注册设备（保持向后兼容）
func (s *Server) handleRegisterDevice(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// handleUnregisterDevice 注销设备（保持向后兼容）
func (s *Server) handleUnregisterDevice(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// handleRelayConnect 处理中继连接
func (s *Server) handleRelayConnect(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// handleRelayData 处理中继数据
func (s *Server) handleRelayData(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// handleHealth 健康检查
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// Load 加载配置
func (c *ServerConfig) Load(filePath string) error {
	return config.LoadConfig(filePath, c)
}
