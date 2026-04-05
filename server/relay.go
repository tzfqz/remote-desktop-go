package main

import (
	"fmt"
	"sync"
)

// RelayServer 中继服务器
type RelayServer struct {
	maxConnections int
	bufferSize     int
	connections    map[string]*RelayConnection
	mu             sync.RWMutex
}

// RelayConnection 中继连接
type RelayConnection struct {
	deviceID string
	sendChan chan []byte
	recvChan chan []byte
}

// NewRelayServer 创建中继服务器
func NewRelayServer(maxConnections, bufferSize int) *RelayServer {
	return &RelayServer{
		maxConnections: maxConnections,
		bufferSize:     bufferSize,
		connections:    make(map[string]*RelayConnection),
	}
}

// Start 启动中继服务器
func (r *RelayServer) Start() {
	// 中继服务器启动逻辑
	fmt.Println("Relay server started")
}

// AddConnection 添加中继连接
func (r *RelayServer) AddConnection(deviceID string) (*RelayConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.connections) >= r.maxConnections {
		return nil, fmt.Errorf("max connections reached")
	}

	// 检查设备是否已存在
	if _, exists := r.connections[deviceID]; exists {
		return nil, fmt.Errorf("device already connected")
	}

	// 创建新的中继连接
	conn := &RelayConnection{
		deviceID: deviceID,
		sendChan: make(chan []byte, r.bufferSize),
		recvChan: make(chan []byte, r.bufferSize),
	}

	r.connections[deviceID] = conn
	return conn, nil
}

// RemoveConnection 移除中继连接
func (r *RelayServer) RemoveConnection(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if conn, exists := r.connections[deviceID]; exists {
		close(conn.sendChan)
		close(conn.recvChan)
		delete(r.connections, deviceID)
	}
}

// GetConnection 获取中继连接
func (r *RelayServer) GetConnection(deviceID string) (*RelayConnection, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	conn, exists := r.connections[deviceID]
	if !exists {
		return nil, fmt.Errorf("device not connected")
	}

	return conn, nil
}

// RelayData 中继数据
func (r *RelayServer) RelayData(sourceID, targetID string, data []byte) error {
	r.mu.RLock()
	targetConn, exists := r.connections[targetID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("target device not connected")
	}

	select {
	case targetConn.recvChan <- data:
		return nil
	default:
		return fmt.Errorf("target buffer full")
	}
}
