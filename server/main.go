package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/cors"

	"remote-desktop/common/config"
)

// Server 服务器结构
type Server struct {
	router *gin.Engine
	config ServerConfig
	relay  *RelayServer
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
	ID   string `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
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
		config: config,
	}

	// 初始化路由器
	server.router = gin.Default()

	// 配置CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
	})
	server.router.Use(func(c *gin.Context) {
		cors.New(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Origin", "Content-Type", "Accept", "Authorization"},
			AllowCredentials: true,
		}).Handler(c.Writer, c.Request)
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
	s.router.POST("/api/devices/register", s.handleRegisterDevice)
	s.router.POST("/api/devices/unregister", s.handleUnregisterDevice)

	// 中继服务
	s.router.POST("/api/relay/connect", s.handleRelayConnect)
	s.router.POST("/api/relay/data", s.handleRelayData)

	// 健康检查
	s.router.GET("/health", s.handleHealth)
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(c *gin.Context) {
	// 实现WebSocket连接处理
	// 这里会处理设备的连接和消息转发
}

// handleGetDevices 获取设备列表
func (s *Server) handleGetDevices(c *gin.Context) {
	// 实现获取设备列表
}

// handleRegisterDevice 注册设备
func (s *Server) handleRegisterDevice(c *gin.Context) {
	// 实现设备注册
}

// handleUnregisterDevice 注销设备
func (s *Server) handleUnregisterDevice(c *gin.Context) {
	// 实现设备注销
}

// handleRelayConnect 处理中继连接
func (s *Server) handleRelayConnect(c *gin.Context) {
	// 实现中继连接
}

// handleRelayData 处理中继数据
func (s *Server) handleRelayData(c *gin.Context) {
	// 实现中继数据传输
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
