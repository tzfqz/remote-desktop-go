package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RelayServer acts as a fallback when P2P connection fails.
// It relays data between agent and controller via WebSocket tunnel.
type RelayServer struct {
	maxConnections int
	bufferSize     int
	connections    map[string]*RelayConnection
	mu             sync.RWMutex
	httpServer     *http.Server
	upgrader       websocket.Upgrader
}

// RelayConnection represents a tunneled connection.
type RelayConnection struct {
	DeviceID   string
	Type       string // "controller" or "agent"
	Conn       *websocket.Conn
	SendChan   chan []byte
	RecvChan   chan []byte
	LastActive time.Time
}

// relayMessage wraps raw data for relay transport.
type relayMessage struct {
	From string `json:"from"`
	To   string `json:"to"`
	Data []byte `json:"data"`
}

// NewRelayServer creates a new relay server.
func NewRelayServer(maxConnections, bufferSize int) *RelayServer {
	return &RelayServer{
		maxConnections: maxConnections,
		bufferSize:     bufferSize,
		connections:    make(map[string]*RelayConnection),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Start launches the relay HTTP+WebSocket server on :8081.
func (r *RelayServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/relay", r.handleRelayWS)
	mux.HandleFunc("/relay/connect", r.handleRelayConnect)

	r.httpServer = &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	go func() {
		log.Println("Relay server listening on :8081")
		if err := r.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Relay server error: %v", err)
		}
	}()
}

// Stop shuts down the relay server.
func (r *RelayServer) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, conn := range r.connections {
		close(conn.SendChan)
		conn.Conn.Close()
	}
	if r.httpServer != nil {
		r.httpServer.Close()
	}
}

func (r *RelayServer) handleRelayWS(w http.ResponseWriter, req *http.Request) {
	conn, err := r.upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Printf("Relay WS upgrade error: %v", err)
		return
	}

	deviceID := req.URL.Query().Get("id")
	deviceType := req.URL.Query().Get("type")

	if deviceID == "" || (deviceType != "controller" && deviceType != "agent") {
		conn.Close()
		return
	}

	r.addConnection(deviceID, deviceType, conn)

	relayConn := r.getConnection(deviceID)
	if relayConn == nil {
		return
	}

	go r.readLoop(relayConn)
	r.writeLoop(relayConn)
}

// readLoop reads messages from the relay connection and forwards them.
func (r *RelayServer) readLoop(conn *RelayConnection) {
	defer conn.Conn.Close()

	for {
		_, message, err := conn.Conn.ReadMessage()
		if err != nil {
			log.Printf("Relay read error from %s: %v", conn.DeviceID, err)
			return
		}
		conn.LastActive = time.Now()

		var rm relayMessage
		if err := json.Unmarshal(message, &rm); err != nil {
			rm = relayMessage{From: conn.DeviceID, To: "", Data: message}
		}

		if rm.To != "" {
			r.forwardTo(rm.To, rm.Data)
		}
	}
}

// writeLoop writes queued messages to the WebSocket.
func (r *RelayServer) writeLoop(conn *RelayConnection) {
	for msg := range conn.SendChan {
		if err := conn.Conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			log.Printf("Relay write error to %s: %v", conn.DeviceID, err)
			return
		}
		conn.LastActive = time.Now()
	}
}

// forwardTo relays data to the target connection.
func (r *RelayServer) forwardTo(targetID string, data []byte) {
	r.mu.RLock()
	conn, exists := r.connections[targetID]
	r.mu.RUnlock()

	if !exists {
		log.Printf("Relay: target %s not found", targetID)
		return
	}

	select {
	case conn.SendChan <- data:
	default:
		log.Printf("Relay: target %s buffer full", targetID)
	}
}

// handleRelayConnect is a health-check endpoint for relay availability.
func (r *RelayServer) handleRelayConnect(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "relay": "available"})
}

// addConnection registers a new relay connection.
func (r *RelayServer) addConnection(deviceID, deviceType string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.connections) >= r.maxConnections {
		conn.Close()
		return
	}

	relayConn := &RelayConnection{
		DeviceID:   deviceID,
		Type:       deviceType,
		Conn:       conn,
		SendChan:   make(chan []byte, r.bufferSize),
		RecvChan:   make(chan []byte, r.bufferSize),
		LastActive: time.Now(),
	}

	r.connections[deviceID] = relayConn
	log.Printf("Relay: connected %s (type=%s), total=%d", deviceID, deviceType, len(r.connections))
}

// removeConnection unregisters a relay connection.
func (r *RelayServer) removeConnection(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if conn, exists := r.connections[deviceID]; exists {
		close(conn.SendChan)
		delete(r.connections, deviceID)
		log.Printf("Relay: disconnected %s, total=%d", deviceID, len(r.connections))
	}
}

// getConnection returns a relay connection by device ID.
func (r *RelayServer) getConnection(deviceID string) *RelayConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[deviceID]
}
