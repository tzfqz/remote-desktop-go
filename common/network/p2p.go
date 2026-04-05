package network

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// P2PConnection P2P连接管理，支持完整的打洞功能
type P2PConnection struct {
	PeerConnection *webrtc.PeerConnection
	DataChannel    *webrtc.DataChannel
	OnData         func([]byte)
	OnError        func(error)
	OnClose        func()
	OnICEConnected func()
	OnICECandidate func(*webrtc.ICECandidateInit)
	candidates     []webrtc.ICECandidate
	candidateMutex sync.Mutex
	isConnected    bool
	connectMutex   sync.Mutex
}

// P2PConfig P2P连接配置
type P2PConfig struct {
	STUNServers []string
	TURNServers []struct {
		URL        string
		Username   string
		Credential string
	}
}

// NewP2PConnection 创建新的P2P连接，带有完整的打洞功能
func NewP2PConnection(config P2PConfig) (*P2PConnection, error) {
	// 配置WebRTC ICE服务器
	iceServers := []webrtc.ICEServer{}

	// 添加STUN服务器
	for _, server := range config.STUNServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs: []string{server},
		})
	}

	// 添加TURN服务器
	for _, server := range config.TURNServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:           []string{server.URL},
			Username:       server.Username,
			Credential:     server.Credential,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	// 配置WebRTC
	rtcConfig := webrtc.Configuration{
		ICEServers: iceServers,
		// 设置ICE传输策略为all，优先使用UDP，然后TCP
		ICETransportPolicy: webrtc.ICETransportPolicyAll,
	}

	// 创建PeerConnection
	peerConnection, err := webrtc.NewPeerConnection(rtcConfig)
	if err != nil {
		return nil, fmt.Errorf("create peer connection error: %v", err)
	}

	p2p := &P2PConnection{
		PeerConnection: peerConnection,
		candidates:     make([]webrtc.ICECandidate, 0),
	}

	// 处理ICE连接状态变化
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		fmt.Printf("ICE connection state: %s\n", state.String())
		
		p2p.connectMutex.Lock()
		defer p2p.connectMutex.Unlock()
		
		switch state {
		case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
			p2p.isConnected = true
			if p2p.OnICEConnected != nil {
				p2p.OnICEConnected()
			}
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateDisconnected:
			p2p.isConnected = false
			if p2p.OnError != nil {
				p2p.OnError(fmt.Errorf("ICE connection failed: %s", state.String()))
			}
		case webrtc.ICEConnectionStateClosed:
			p2p.isConnected = false
			if p2p.OnClose != nil {
				p2p.OnClose()
			}
		}
	})

	// 处理ICE候选收集 - 打洞的关键
	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			// 所有候选收集完毕
			fmt.Println("All ICE candidates collected")
			return
		}

		fmt.Printf("Collected ICE candidate: %s\n", candidate.Address)

		// 存储候选
		p2p.candidateMutex.Lock()
		p2p.candidates = append(p2p.candidates, *candidate)
		p2p.candidateMutex.Unlock()

		// 发送候选到信令服务器
		if p2p.OnICECandidate != nil {
			candidateInit := candidate.ToJSON()
			p2p.OnICECandidate(&candidateInit)
		}
	})

	// 处理连接状态变化
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer connection state: %s\n", state.String())
	})

	// 处理数据通道
	peerConnection.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Printf("Data channel opened: %s\n", dc.Label())
		p2p.DataChannel = dc
		p2p.setupDataChannel(dc)
	})

	return p2p, nil
}

// setupDataChannel 设置数据通道
func (p *P2PConnection) setupDataChannel(dc *webrtc.DataChannel) {
	dc.OnOpen(func() {
		fmt.Println("Data channel opened")
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if p.OnData != nil {
			p.OnData(msg.Data)
		}
	})

	dc.OnClose(func() {
		fmt.Println("Data channel closed")
		if p.OnClose != nil {
			p.OnClose()
		}
	})

	dc.OnError(func(err error) {
		fmt.Printf("Data channel error: %v\n", err)
		if p.OnError != nil {
			p.OnError(err)
		}
	})
}

// CreateOffer 创建连接请求，开始打洞过程
func (p *P2PConnection) CreateOffer() (string, error) {
	// 创建数据通道
	dc, err := p.PeerConnection.CreateDataChannel("remote-desktop", nil)
	if err != nil {
		return "", fmt.Errorf("create data channel error: %v", err)
	}

	p.setupDataChannel(dc)
	p.DataChannel = dc

	// 创建offer
	offer, err := p.PeerConnection.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("create offer error: %v", err)
	}

	// 设置本地描述
	err = p.PeerConnection.SetLocalDescription(offer)
	if err != nil {
		return "", fmt.Errorf("set local description error: %v", err)
	}

	// 等待ICE候选收集（超时2秒）
	fmt.Println("Waiting for ICE candidates...")
	time.Sleep(2 * time.Second)

	// 获取本地描述的JSON
	localDesc := p.PeerConnection.LocalDescription()
	if localDesc == nil {
		return "", fmt.Errorf("local description is nil")
	}

	return localDesc.SDP, nil
}

// SetRemoteAnswer 设置远程应答
func (p *P2PConnection) SetRemoteAnswer(answer string) error {
	rdesc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answer,
	}

	err := p.PeerConnection.SetRemoteDescription(rdesc)
	if err != nil {
		return fmt.Errorf("set remote description error: %v", err)
	}

	return nil
}

// SetRemoteOffer 设置远程请求
func (p *P2PConnection) SetRemoteOffer(offer string) (string, error) {
	rdesc := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offer,
	}

	err := p.PeerConnection.SetRemoteDescription(rdesc)
	if err != nil {
		return "", fmt.Errorf("set remote description error: %v", err)
	}

	// 创建应答
	answer, err := p.PeerConnection.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("create answer error: %v", err)
	}

	// 设置本地描述
	err = p.PeerConnection.SetLocalDescription(answer)
	if err != nil {
		return "", fmt.Errorf("set local description error: %v", err)
	}

	// 等待ICE候选收集（超时2秒）
	fmt.Println("Waiting for ICE candidates...")
	time.Sleep(2 * time.Second)

	// 获取本地描述的JSON
	localDesc := p.PeerConnection.LocalDescription()
	if localDesc == nil {
		return "", fmt.Errorf("local description is nil")
	}

	return localDesc.SDP, nil
}

// AddICECandidate 添加远程ICE候选 - 打洞的关键步骤
func (p *P2PConnection) AddICECandidate(candidateInit webrtc.ICECandidateInit) error {
	fmt.Printf("Adding ICE candidate: %s\n", candidateInit.Candidate)
	return p.PeerConnection.AddICECandidate(candidateInit)
}

// GetCollectedCandidates 获取收集到的ICE候选
func (p *P2PConnection) GetCollectedCandidates() []webrtc.ICECandidate {
	p.candidateMutex.Lock()
	defer p.candidateMutex.Unlock()
	
	candidates := make([]webrtc.ICECandidate, len(p.candidates))
	copy(candidates, p.candidates)
	return candidates
}

// IsConnected 检查是否已连接
func (p *P2PConnection) IsConnected() bool {
	p.connectMutex.Lock()
	defer p.connectMutex.Unlock()
	return p.isConnected
}

// WaitForConnected 等待连接建立
func (p *P2PConnection) WaitForConnected(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	checkInterval := 100 * time.Millisecond
	
	for time.Now().Before(deadline) {
		if p.IsConnected() {
			return nil
		}
		time.Sleep(checkInterval)
	}
	
	return fmt.Errorf("connection timeout after %v", timeout)
}

// Send 发送数据
func (p *P2PConnection) Send(data []byte) error {
	if p.DataChannel == nil {
		return fmt.Errorf("data channel is not ready")
	}
	return p.DataChannel.Send(data)
}

// SendJSON 发送JSON数据
func (p *P2PConnection) SendJSON(obj interface{}) error {
	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return p.Send(data)
}

// Close 关闭连接
func (p *P2PConnection) Close() error {
	if p.PeerConnection != nil {
		return p.PeerConnection.Close()
	}
	return nil
}

// GetConnectionStats 获取连接统计信息
func (p *P2PConnection) GetConnectionStats() (webrtc.StatsReport, error) {
	return p.PeerConnection.GetStats(), nil
}

// GetNATType 获取NAT类型（根据ICE候选判断）
func (p *P2PConnection) GetNATType() string {
	candidates := p.GetCollectedCandidates()
	if len(candidates) == 0 {
		return "unknown"
	}

	hasHost := false
	hasServerReflexive := false
	hasRelay := false

	for _, candidate := range candidates {
		switch candidate.Typ {
		case webrtc.ICECandidateTypeHost:
			hasHost = true
		case webrtc.ICECandidateTypeSrflx:
			hasServerReflexive = true
		case webrtc.ICECandidateTypeRelay:
			hasRelay = true
		}
	}

	// 简单的NAT类型判断
	if hasHost && !hasServerReflexive && !hasRelay {
		return "open_internet"
	}
	if hasHost && hasServerReflexive && !hasRelay {
		return "full_cone"
	}
	if hasServerReflexive && !hasHost && !hasRelay {
		return "symmetric"
	}
	if hasRelay {
		return "symmetric_udp_blocked"
	}
	return "unknown"
}
