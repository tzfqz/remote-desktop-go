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

// Controller 控制端结构
type Controller struct {
	config     ControllerConfig
	ws         *websocket.Conn
	p2pConn    *network.P2PConnection
	isRunning  bool
}

// ControllerConfig 控制端配置
type ControllerConfig struct {
	Server struct {
		URL              string `yaml:"url"`
		ReconnectInterval int    `yaml:"reconnect_interval"`
	} `yaml:"server"`
	Network struct {
		P2P       bool     `yaml:"p2p"`
		Relay     bool     `yaml:"relay"`
	ICEServers []string `yaml:"ice_servers"`
	} `yaml:"network"`
	Display struct {
		FPS        int  `yaml:"fps"`
		Quality    int  `yaml:"quality"`
		Resize     bool `yaml:"resize"`
		Fullscreen bool `yaml:"fullscreen"`
	} `yaml:"display"`
	Control struct {
		EnableKeyboard     bool    `yaml:"enable_keyboard"`
		EnableMouse        bool    `yaml:"enable_mouse"`
		EnableClipboard    bool    `yaml:"enable_clipboard"`
		MouseSensitivity   float64 `yaml:"mouse_sensitivity"`
	} `yaml:"control"`
	Security struct {
		AuthToken string `yaml:"auth_token"`
	} `yaml:"security"`
	Logging struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"logging"`
}

// main 主函数
func main() {
	// 加载配置
	var config ControllerConfig
	err := config.Load("controller_config.yaml")
	if err != nil {
		log.Fatalf("Load config error: %v", err)
	}

	// 创建控制端
	controller := &Controller{
		config:    config,
		isRunning: true,
	}

	// 连接服务器
	err = controller.connectServer()
	if err != nil {
		log.Fatalf("Connect server error: %v", err)
	}

	// 处理信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 主循环
	go controller.run()

	// 等待信号
	<-sigChan
	fmt.Println("Received signal, exiting...")
	controller.isRunning = false

	// 清理资源
	controller.cleanup()
}

// Load 加载配置
func (c *ControllerConfig) Load(filePath string) error {
	return config.LoadConfig(filePath, c)
}

// connectServer 连接服务器
func (c *Controller) connectServer() error {
	// 连接WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(c.config.Server.URL, nil)
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}

	c.ws = conn
	fmt.Println("Connected to server")

	// 发送加入消息
	joinMsg := protocol.Message{
		Type:    protocol.MsgTypeJoin,
		Payload: map[string]string{"id": "controller-" + fmt.Sprintf("%d", time.Now().Unix()), "name": "Controller"},
	}
	data, _ := json.Marshal(joinMsg)
	err = c.ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		return fmt.Errorf("send join message error: %v", err)
	}

	// 启动接收协程
	go c.receiveLoop()

	return nil
}

// receiveLoop 接收消息循环
func (c *Controller) receiveLoop() {
	defer func() {
		if c.ws != nil {
			c.ws.Close()
		}
	}()

	for c.isRunning {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			log.Printf("Read error: %v", err)
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
		c.handleMessage(&msg)
	}
}

// handleMessage 处理消息
func (c *Controller) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.MsgTypeDeviceList:
		c.handleDeviceList(msg)
	case protocol.MsgTypeAnswer:
		c.handleAnswer(msg)
	case protocol.MsgTypeCandidate:
		c.handleCandidate(msg)
	case protocol.MsgTypeScreenFrame:
		c.handleScreenFrame(msg)
	}
}

// handleDeviceList 处理设备列表
func (c *Controller) handleDeviceList(msg *protocol.Message) {
	// 解析设备列表
	devices, ok := msg.Payload.([]interface{})
	if !ok {
		return
	}

	fmt.Println("Available devices:")
	for i, device := range devices {
		deviceMap, ok := device.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := deviceMap["id"].(string)
		name, _ := deviceMap["name"].(string)
		fmt.Printf("%d. %s (ID: %s)\n", i+1, name, id)
	}

	// 这里可以添加选择设备的逻辑
}

// connectToDevice 连接到设备
func (c *Controller) connectToDevice(deviceID string) error {
	// 创建P2P连接
	p2pConfig := network.P2PConfig{
		STUNServers: c.config.Network.ICEServers,
	}

	p2pConn, err := network.NewP2PConnection(p2pConfig)
	if err != nil {
		return fmt.Errorf("create P2P connection error: %v", err)
	}

	// 设置回调
	p2pConn.OnData = func(data []byte) {
		c.handleP2PData(data)
	}
	p2pConn.OnError = func(err error) {
		log.Printf("P2P error: %v", err)
	}
	p2pConn.OnClose = func() {
		log.Println("P2P connection closed")
		c.p2pConn = nil
	}
	p2pConn.OnICEConnected = func() {
		log.Println("P2P connection established")
	}
	p2pConn.OnICECandidate = func(candidate *webrtc.ICECandidateInit) {
		c.sendICECandidate(deviceID, candidate)
	}

	c.p2pConn = p2pConn

	// 创建offer
	offer, err := p2pConn.CreateOffer()
	if err != nil {
		return fmt.Errorf("create offer error: %v", err)
	}

	// 发送offer
	offerMsg := protocol.Message{
		Type: protocol.MsgTypeOffer,
		To:   deviceID,
		Payload: protocol.OfferPayload{
			SDP: offer,
		},
	}
	c.sendSignalingMessage(offerMsg)

	return nil
}

// handleAnswer 处理应答
func (c *Controller) handleAnswer(msg *protocol.Message) {
	if c.p2pConn == nil {
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
	err := c.p2pConn.SetRemoteAnswer(sdp)
	if err != nil {
		log.Printf("Set remote answer error: %v", err)
	}
}

// handleCandidate 处理ICE候选
func (c *Controller) handleCandidate(msg *protocol.Message) {
	if c.p2pConn == nil {
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
	err := c.p2pConn.AddICECandidate(candidate)
	if err != nil {
		log.Printf("Add ICE candidate error: %v", err)
	}
}

// sendICECandidate 发送ICE候选
func (c *Controller) sendICECandidate(to string, candidate *webrtc.ICECandidateInit) {
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
	c.sendSignalingMessage(candidateMsg)
}

// handleP2PData 处理P2P数据
func (c *Controller) handleP2PData(data []byte) {
	// 解析消息
	var msg protocol.Message
	err := json.Unmarshal(data, &msg)
	if err != nil {
		log.Printf("Parse P2P message error: %v", err)
		return
	}

	// 处理数据消息
	switch msg.Type {
	case protocol.MsgTypeScreenFrame:
		c.handleScreenFrame(&msg)
	}
}

// handleScreenFrame 处理屏幕帧
func (c *Controller) handleScreenFrame(msg *protocol.Message) {
	// 解析屏幕帧
	_, ok := msg.Payload.(map[string]interface{})
	if !ok {
		return
	}

	// 处理屏幕数据
	// 这里可以添加显示逻辑
}

// sendControlMessage 发送控制消息
func (c *Controller) sendControlMessage(msg protocol.Message) {
	if c.p2pConn != nil && c.p2pConn.IsConnected() {
		c.p2pConn.SendJSON(msg)
	}
}

// run 运行主循环
func (c *Controller) run() {
	// 主循环逻辑
	// 这里可以添加用户输入处理
}

// sendSignalingMessage 发送信令消息
func (c *Controller) sendSignalingMessage(msg protocol.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Marshal message error: %v", err)
		return
	}

	err = c.ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		log.Printf("Send message error: %v", err)
	}
}

// cleanup 清理资源
func (c *Controller) cleanup() {
	if c.p2pConn != nil {
		c.p2pConn.Close()
	}
	if c.ws != nil {
		c.ws.Close()
	}
	fmt.Println("Cleaned up resources")
}
