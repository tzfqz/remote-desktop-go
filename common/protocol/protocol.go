package protocol

// MessageType 消息类型
type MessageType string

const (
	// 信令消息
	MsgTypeOffer      MessageType = "offer"
	MsgTypeAnswer     MessageType = "answer"
	MsgTypeCandidate  MessageType = "candidate"
	MsgTypeJoin       MessageType = "join"
	MsgTypeLeave      MessageType = "leave"
	MsgTypeDeviceList MessageType = "device_list"
	MsgTypeGetDevices MessageType = "get_devices"
	
	// 控制消息
	MsgTypeMouseMove   MessageType = "mouse_move"
	MsgTypeMouseClick  MessageType = "mouse_click"
	MsgTypeMouseScroll MessageType = "mouse_scroll"
	MsgTypeKeyDown     MessageType = "key_down"
	MsgTypeKeyUp       MessageType = "key_up"
	
	// 数据消息
	MsgTypeScreenFrame MessageType = "screen_frame"
	MsgTypeClipboard   MessageType = "clipboard"
	MsgTypeError       MessageType = "error"
)

// Message 通用消息结构
type Message struct {
	Type    MessageType `json:"type"`
	From    string      `json:"from,omitempty"`
	To      string      `json:"to,omitempty"`
	Payload interface{} `json:"payload"`
}

// OfferPayload Offer消息载荷
type OfferPayload struct {
	SDP string `json:"sdp"`
}

// AnswerPayload Answer消息载荷
type AnswerPayload struct {
	SDP string `json:"sdp"`
}

// CandidatePayload ICE候选消息载荷
type CandidatePayload struct {
	Candidate string `json:"candidate"`
	SDPMLine  int    `json:"sdp_m_line"`
	SDPMid    string `json:"sdp_mid"`
}

// MouseMovePayload 鼠标移动载荷
type MouseMovePayload struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MouseClickPayload 鼠标点击载荷
type MouseClickPayload struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Button int     `json:"button"`
	Down   bool    `json:"down"`
}

// MouseScrollPayload 鼠标滚动载荷
type MouseScrollPayload struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	DeltaX   int     `json:"delta_x"`
	DeltaY   int     `json:"delta_y"`
}

// KeyPayload 键盘载荷
type KeyPayload struct {
	KeyCode int    `json:"key_code"`
	Key     string `json:"key,omitempty"`
}

// ScreenFramePayload 屏幕帧载荷
// Data 为 base64 编码字符串（便于跨语言 JSON 序列化）
type ScreenFramePayload struct {
	Data      string `json:"data"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Timestamp int64  `json:"timestamp"`
}

// ClipboardPayload 剪贴板载荷
type ClipboardPayload struct {
	Text string `json:"text"`
}

// ErrorPayload 错误载荷
type ErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
