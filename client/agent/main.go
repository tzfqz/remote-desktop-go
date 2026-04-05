package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"

	"remote-desktop/common/config"
	"remote-desktop/common/network"
	"remote-desktop/common/protocol"
)

// Agent 被控端结构
type Agent struct {
	config     AgentConfig
	ws         *websocket.Conn
	p2pConn    *network.P2PConnection
	isRunning  bool
	screenCap  *ScreenCapture
	inputCtrl  *InputController
}

// AgentConfig 被控端配置
type AgentConfig struct {
	Server struct {
		URL              string `yaml:"url"`
		ReconnectInterval int    `yaml:"reconnect_interval"`
	} `yaml:"server"`
	Device struct {
		Name string `yaml:"name"`
		ID   string `yaml:"id"`
	} `yaml:"device"`
	Screen struct {
		FPS            int    `yaml:"fps"`
		Quality        int    `yaml:"quality"`
		CaptureMethod  string `yaml:"capture_method"`
	} `yaml:"screen"`
	Control struct {
		EnableKeyboard  bool `yaml:"enable_keyboard"`
		EnableMouse     bool `yaml:"enable_mouse"`
		EnableClipboard bool `yaml:"enable_clipboard"`
	} `yaml:"control"`
	Network struct {
		P2P       bool     `yaml:"p2p"`
		Relay     bool     `yaml:"relay"`
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

// ScreenCapture 屏幕捕获
type ScreenCapture struct {
	fps            int
	quality        int
	captureMethod  string
}

// InputController 输入控制器
type InputController struct {
	enableKeyboard  bool
	enableMouse     bool
	enableClipboard bool
}

// main 主函数
func main() {
	// 加载配置
	var config AgentConfig
	err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Load config error: %v", err)
	}

	// 创建被控端
	agent := &Agent{
		config:    config,
		isRunning: true,
	}

	// 初始化屏幕捕获
	agent.screenCap = NewScreenCapture(config.Screen.FPS, config.Screen.Quality, config.Screen.CaptureMethod)

	// 初始化输入控制器
	agent.inputCtrl = NewInputController(config.Control.EnableKeyboard, config.Control.EnableMouse, config.Control.EnableClipboard)

	// 连接服务器
	err = agent.connectServer()
	if err != nil {
		log.Fatalf("Connect server error: %v", err)
	}

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 主循环
	go agent.run()

	// 等待信号
	<-sigChan
	fmt.Println("Received signal, exiting...")
	agent.isRunning = false

	// 清理资源
	agent.cleanup()
}

// Load 加载配置
func (c *AgentConfig) Load(filePath string) error {
	return config.LoadConfig(filePath, c)
}

// connectServer 连接服务器
func (a *Agent) connectServer() error {
	// 连接WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(a.config.Server.URL, nil)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	a.ws = conn
	fmt.Println("Connected to server")

	// 发送加入消息
	joinMsg := protocol.Message{
		Type:    protocol.MsgTypeJoin,
		Payload: map[string]string{"id": a.config.Device.ID, "name": a.config.Device.Name},
	}
	data, _ := json.Marshal(joinMsg)
	err = a.ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return fmt.Errorf("send join message error: %v", err)
	}

	// 启动接收协程
	go a.receiveLoop()

	return nil
}

// receiveLoop 接收消息循环
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
			// 重连
			time.Sleep(time.Duration(a.config.Server.ReconnectInterval) * time.Second)
			a.connectServer()
			return
		}

		// 解析消息
		var msg protocol.Message
		err = json.Unmarshal(message, &msg)
		if err != nil {
			log.Printf("Parse message error: %v", err)
			continue
		}

		// 处理消息
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
	case protocol.MsgTypeMouseMove:
		a.handleMouseMove(msg)
	case protocol.MsgTypeMouseClick:
		a.handleMouseClick(msg)
	case protocol.MsgTypeMouseScroll:
		a.handleMouseScroll(msg)
	case protocol.MsgTypeKeyDown:
		a.handleKeyDown(msg)
	case protocol.MsgTypeKeyUp:
		a.handleKeyUp(msg)
	}
}

// handleOffer 处理连接请求
func (a *Agent) handleOffer(msg *protocol.Message) {
	// 解析offer
	offerPayload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	sdp, ok := offerPayload["sdp"].(string)
	if !ok {
		return
	}

	// 创建P2P连接
	p2pConfig := network.P2PConfig{
		STUNServers: a.config.Network.ICEServers,
	}

	p2pConn, err := network.NewP2PConnection(p2pConfig)
	if err != nil {
		log.Printf("Create P2P connection error: %v", err)
		return
	}

	// 设置回调
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
	p2pConn.OnICECandidate = func(candidate *webrtc.ICECandidateInit) {
		a.sendICECandidate(msg.From, candidate)
	}

	a.p2pConn = p2pConn

	// 处理offer
	answer, err := p2pConn.SetRemoteOffer(sdp)
	if err != nil {
		log.Printf("Set remote offer error: %v", err)
		return
	}

	// 发送answer
	answerMsg := protocol.Message{
		Type: protocol.MsgTypeAnswer,
		To:   msg.From,
		Payload: protocol.AnswerPayload{
			SDP: answer,
		},
	}
	a.sendSignalingMessage(answerMsg)
}

// handleAnswer 处理应答
func (a *Agent) handleAnswer(msg *protocol.Message) {
	if a.p2pConn == nil {
		return
	}

	// 解析answer
	answerPayload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}
	sdp, ok := answerPayload["sdp"].(string)
	if !ok {
		return
	}

	// 设置远程应答
	err := a.p2pConn.SetRemoteAnswer(sdp)
	if err != nil {
		log.Printf("Set remote answer error: %v", err)
	}
}

// handleCandidate 处理ICE候选
func (a *Agent) handleCandidate(msg *protocol.Message) {
	if a.p2pConn == nil {
		return
	}

	// 解析候选
	candidatePayload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	candidate := webrtc.ICECandidateInit{
		Candidate: candidatePayload["candidate"].(string),
	}
	if sdpMid, ok := candidatePayload["sdp_mid"].(string); ok {
		candidate.SDPMid = &sdpMid
	}
	if sdpMLine, ok := candidatePayload["sdp_m_line"].(float64); ok {
		sdpMLineIndex := uint16(sdpMLine)
		candidate.SDPMLineIndex = &sdpMLineIndex
	}

	// 添加候选
	err := a.p2pConn.AddICECandidate(candidate)
	if err != nil {
		log.Printf("Add ICE candidate error: %v", err)
	}
}

// sendICECandidate 发送ICE候选
func (a *Agent) sendICECandidate(to string, candidate *webrtc.ICECandidateInit) {
	sdpMid := ""
	if candidate.SDPMid != nil {
		sdpMid = *candidate.SDPMid
	}
	sdpMLine := 0
	if candidate.SDPMLineIndex != nil {
		sdpMLine = int(*candidate.SDPMLineIndex)
	}

	candidateMsg := protocol.Message{
		Type: protocol.MsgTypeCandidate,
		To:   to,
		Payload: protocol.CandidatePayload{
			Candidate: candidate.Candidate,
			SDPMid:    sdpMid,
			SDPMLine:  sdpMLine,
		},
	}
	a.sendSignalingMessage(candidateMsg)
}

// handleP2PData 处理P2P数据
func (a *Agent) handleP2PData(data []byte) {
	// 解析消息
	var msg protocol.Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		log.Printf("Parse P2P message error: %v", err)
		return
	}

	// 处理控制消息
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
	// 实现鼠标移动
}

// handleMouseClick 处理鼠标点击
func (a *Agent) handleMouseClick(msg *protocol.Message) {
	// 实现鼠标点击
}

// handleMouseScroll 处理鼠标滚动
func (a *Agent) handleMouseScroll(msg *protocol.Message) {
	// 实现鼠标滚动
}

// handleKeyDown 处理键盘按下
func (a *Agent) handleKeyDown(msg *protocol.Message) {
	// 实现键盘按下
}

// handleKeyUp 处理键盘释放
func (a *Agent) handleKeyUp(msg *protocol.Message) {
	// 实现键盘释放
}

// run 运行主循环
func (a *Agent) run() {
	ticker := time.NewTicker(time.Duration(1000/a.config.Screen.FPS) * time.Millisecond)
	defer ticker.Stop()

	for a.isRunning {
		select {
		case <-ticker.C:
			// 捕获屏幕
			frame, err := a.screenCap.Capture()
			if err != nil {
				log.Printf("Capture screen error: %v", err)
				continue
			}

			// 发送屏幕数据
			if a.p2pConn != nil && a.p2pConn.IsConnected() {
				screenMsg := protocol.Message{
					Type: protocol.MsgTypeScreenFrame,
					Payload: protocol.ScreenFramePayload{
						Data:      frame,
						Width:     1920,
						Height:    1080,
						Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
					},
				}
				a.p2pConn.SendJSON(screenMsg)
			}
		}
	}
}

// sendSignalingMessage 发送信令消息
func (a *Agent) sendSignalingMessage(msg protocol.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Marshal message error: %v", err)
		return
	}

	err = a.ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		log.Printf("Send message error: %v", err)
	}
}

// cleanup 清理资源
func (a *Agent) cleanup() {
	if a.p2pConn != nil {
		a.p2pConn.Close()
	}
	if a.ws != nil {
		a.ws.Close()
	}
	fmt.Println("Cleaned up resources")
}

// NewScreenCapture 创建屏幕捕获
func NewScreenCapture(fps, quality int, captureMethod string) *ScreenCapture {
	return &ScreenCapture{
		fps:            fps,
		quality:        quality,
		captureMethod:  captureMethod,
	}
}

// Capture 捕获屏幕
func (sc *ScreenCapture) Capture() ([]byte, error) {
	// 实现屏幕捕获
	// 这里返回模拟数据
	return []byte("mock screen data"), nil
}

// NewInputController 创建输入控制器
func NewInputController(enableKeyboard, enableMouse, enableClipboard bool) *InputController {
	return &InputController{
		enableKeyboard:  enableKeyboard,
		enableMouse:     enableMouse,
		enableClipboard: enableClipboard,
	}
}
