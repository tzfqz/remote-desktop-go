package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"

	"remote-desktop/client/agent/input"
	"remote-desktop/client/agent/screen"
	"remote-desktop/common/config"
	"remote-desktop/common/network"
	"remote-desktop/common/protocol"
)

// Agent 被控端结构
type Agent struct {
	config    AgentConfig
	ws        *websocket.Conn
	p2pConn   *network.P2PConnection
	isRunning bool
	capture   *screen.CaptureGDI
	inputCtrl *input.InputController
}

// AgentConfig 被控端配置
type AgentConfig struct {
	Server struct {
		URL               string `yaml:"url"`
		ReconnectInterval int    `yaml:"reconnect_interval"`
	} `yaml:"server"`
	Device struct {
		Name string `yaml:"name"`
		ID   string `yaml:"id"`
	} `yaml:"device"`
	Screen struct {
		FPS           int    `yaml:"fps"`
		Quality       int    `yaml:"quality"`
		CaptureMethod string `yaml:"capture_method"`
		Width         int    `yaml:"width"`
		Height        int    `yaml:"height"`
	} `yaml:"screen"`
	Control struct {
		EnableKeyboard  bool `yaml:"enable_keyboard"`
		EnableMouse     bool `yaml:"enable_mouse"`
		EnableClipboard bool `yaml:"enable_clipboard"`
	} `yaml:"control"`
	Network struct {
		P2P         bool     `yaml:"p2p"`
		Relay       bool     `yaml:"relay"`
		ICEServers []string `yaml:"ice_servers"`
	} `yaml:"network"`
	Security struct {
		AuthToken string `yaml:"auth_token"`
	} `yaml:"security"`
	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`
}

func main() {
	var config AgentConfig
	if err := config.Load("agent_config.yaml"); err != nil {
		log.Fatalf("Load config error: %v", err)
	}

	agent := &Agent{
		config:    config,
		isRunning: true,
	}

	// 初始化屏幕捕获
	sc, err := screen.NewCaptureGDI(config.Screen.Width, config.Screen.Height, config.Screen.Quality)
	if err != nil {
		log.Printf("WARN: GDI capture init failed: %v, using stub", err)
		sc = nil
	}
	agent.capture = sc

	// 初始化输入控制器
	agent.inputCtrl = input.NewInputController(config.Control.EnableKeyboard, config.Control.EnableMouse)

	// 连接服务器
	if err := agent.connectServer(); err != nil {
		log.Fatalf("Connect server error: %v", err)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go agent.run()

	<-sigChan
	fmt.Println("Exiting...")
	agent.isRunning = false
	agent.cleanup()
}

// Load 加载配置
func (c *AgentConfig) Load(filePath string) error {
	return config.LoadConfig(filePath, c)
}

// connectServer 连接服务器
func (a *Agent) connectServer() error {
	wsURL := a.config.Server.URL
	// 如果 URL 没有查询参数则加 ?
	if !strings.Contains(wsURL, "?") {
		wsURL += "?"
	} else {
		wsURL += "&"
	}
	wsURL += "id=" + a.config.Device.ID + "&type=agent"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}
	a.ws = conn
	fmt.Println("Connected to server")

	joinMsg := protocol.Message{
		Type:    protocol.MsgTypeJoin,
		Payload: map[string]string{"id": a.config.Device.ID, "name": a.config.Device.Name},
	}
	data, _ := json.Marshal(joinMsg)
	if err := a.ws.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("send join error: %v", err)
	}

	go a.receiveLoop()
	return nil
}

// receiveLoop 接收信令消息
func (a *Agent) receiveLoop() {
	defer func() {
		if a.ws != nil {
			a.ws.Close()
		}
	}()

	for a.isRunning {
		_, message, err := a.ws.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
			time.Sleep(time.Duration(a.config.Server.ReconnectInterval) * time.Second)
			a.connectServer()
			return
		}

		var msg protocol.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Parse error: %v", err)
			continue
		}
		a.handleMessage(&msg)
	}
}

// handleMessage 处理消息
func (a *Agent) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MsgTypeOffer:
		a.handleOffer(msg)
	case protocol.MsgTypeAnswer:
		a.handleAnswer(msg)
	case protocol.MsgTypeCandidate:
		a.handleCandidate(msg)
	}
}

// handleOffer 处理 WebRTC Offer
func (a *Agent) handleOffer(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	sdp, ok := payload["sdp"].(string)
	if !ok {
		return
	}

	p2pConfig := network.P2PConfig{
		STUNServers: a.config.Network.ICEServers,
	}
	p2pConn, err := network.NewP2PConnection(p2pConfig)
	if err != nil {
		log.Printf("Create P2P error: %v", err)
		return
	}

	p2pConn.OnData = func(data []byte) {
		a.handleP2PData(data)
	}
	p2pConn.OnError = func(err error) {
		log.Printf("P2P error: %v", err)
	}
	p2pConn.OnClose = func() {
		log.Println("P2P connection closed")
		a.p2pConn = nil
	}
	p2pConn.OnICEConnected = func() {
		log.Println("P2P connection established")
	}
	p2pConn.OnICECandidate = func(c *webrtc.ICECandidateInit) {
		a.sendICECandidate(msg.From, c)
	}
	a.p2pConn = p2pConn

	answer, err := p2pConn.SetRemoteOffer(sdp)
	if err != nil {
		log.Printf("Set remote offer error: %v", err)
		return
	}

	answerMsg := protocol.Message{
		Type: protocol.MsgTypeAnswer,
		To:   msg.From,
		Payload: protocol.AnswerPayload{
			SDP: answer,
		},
	}
	a.sendSignalingMessage(answerMsg)
}

// handleAnswer 处理 WebRTC Answer
func (a *Agent) handleAnswer(msg *protocol.Message) {
	if a.p2pConn == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	sdp, ok := payload["sdp"].(string)
	if !ok {
		return
	}
	if err := a.p2pConn.SetRemoteAnswer(sdp); err != nil {
		log.Printf("Set remote answer error: %v", err)
	}
}

// handleCandidate 处理 ICE Candidate
func (a *Agent) handleCandidate(msg *protocol.Message) {
	if a.p2pConn == nil {
		return
	}
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	c := webrtc.ICECandidateInit{Candidate: payload["candidate"].(string)}
	if s, ok := payload["sdp_mid"].(string); ok {
		c.SDPMid = &s
	}
	if m, ok := payload["sdp_m_line"].(float64); ok {
		i := uint16(m)
		c.SDPMLineIndex = &i
	}
	if err := a.p2pConn.AddICECandidate(c); err != nil {
		log.Printf("Add ICE candidate error: %v", err)
	}
}

// handleP2PData 处理 P2P 数据通道消息
func (a *Agent) handleP2PData(data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Type {
	case protocol.MsgTypeMouseMove:
		a.handleMouseMove(&msg)
	case protocol.MsgTypeMouseClick:
		a.handleMouseClick(&msg)
	case protocol.MsgTypeMouseScroll:
		a.handleMouseScroll(&msg)
	case protocol.MsgTypeKeyDown:
		a.handleKeyDown(&msg)
	case protocol.MsgTypeKeyUp:
		a.handleKeyUp(&msg)
	}
}

// handleMouseMove 处理鼠标移动
func (a *Agent) handleMouseMove(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	x, _ := payload["x"].(float64)
	y, _ := payload["y"].(float64)
	if a.inputCtrl != nil {
		a.inputCtrl.MoveMouse(x, y)
	}
}

// handleMouseClick 处理鼠标点击
func (a *Agent) handleMouseClick(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	x, _ := payload["x"].(float64)
	y, _ := payload["y"].(float64)
	button, _ := payload["button"].(float64)
	down, _ := payload["down"].(bool)
	if a.inputCtrl != nil {
		a.inputCtrl.MouseClick(x, y, int(button), down)
	}
}

// handleMouseScroll 处理鼠标滚动
func (a *Agent) handleMouseScroll(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	x, _ := payload["x"].(float64)
	y, _ := payload["y"].(float64)
	dx, _ := payload["delta_x"].(float64)
	dy, _ := payload["delta_y"].(float64)
	if a.inputCtrl != nil {
		a.inputCtrl.MouseScroll(x, y, int(dx), int(dy))
	}
}

// handleKeyDown 处理键盘按下
func (a *Agent) handleKeyDown(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	kc, _ := payload["key_code"].(float64)
	key, _ := payload["key"].(string)
	if a.inputCtrl != nil {
		a.inputCtrl.KeyDown(int(kc), key)
	}
}

// handleKeyUp 处理键盘释放
func (a *Agent) handleKeyUp(msg *protocol.Message) {
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	kc, _ := payload["key_code"].(float64)
	key, _ := payload["key"].(string)
	if a.inputCtrl != nil {
		a.inputCtrl.KeyUp(int(kc), key)
	}
}

// run 主循环：定时捕获屏幕并通过 P2P 发送
func (a *Agent) run() {
	interval := time.Duration(1000/a.config.Screen.FPS) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	frameCount := 0
	for a.isRunning {
		<-ticker.C
		if a.capture == nil {
			continue
		}
		b64, w, h, err := a.capture.Capture()
		if err != nil {
			log.Printf("Capture error: %v", err)
			continue
		}
		frameCount++

		if a.p2pConn != nil && a.p2pConn.IsConnected() {
			screenMsg := protocol.Message{
				Type: protocol.MsgTypeScreenFrame,
				Payload: protocol.ScreenFramePayload{
					Data:      b64, // base64 编码字符串
					Width:     w,
					Height:    h,
					Timestamp: time.Now().UnixMilli(),
				},
			}
			if err := a.p2pConn.SendJSON(screenMsg); err != nil {
				log.Printf("Send screen frame error: %v", err)
			}
		}
	}
}

// sendSignalingMessage 发送信令消息
func (a *Agent) sendSignalingMessage(msg protocol.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Marshal error: %v", err)
		return
	}
	if err := a.ws.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("Send error: %v", err)
	}
}

// sendICECandidate 发送 ICE 候选到信令服务器
func (a *Agent) sendICECandidate(to string, c *webrtc.ICECandidateInit) {
	sdpMid := ""
	if c.SDPMid != nil {
		sdpMid = *c.SDPMid
	}
	sdpMLine := 0
	if c.SDPMLineIndex != nil {
		sdpMLine = int(*c.SDPMLineIndex)
	}
	msg := protocol.Message{
		Type: protocol.MsgTypeCandidate,
		To:   to,
		Payload: protocol.CandidatePayload{
			Candidate: c.Candidate,
			SDPMid:   sdpMid,
			SDPMLine: sdpMLine,
		},
	}
	a.sendSignalingMessage(msg)
}

// cleanup 清理资源
func (a *Agent) cleanup() {
	if a.capture != nil {
		a.capture.Close()
	}
	if a.p2pConn != nil {
		a.p2pConn.Close()
	}
	if a.ws != nil {
		a.ws.Close()
	}
	fmt.Println("Cleaned up resources")
}
