package ws

// 这个文件实现了hybi草案的协议

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
)

const (
	// 用于生成Sec-WebSocket-Accpet
	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// 关闭原因
	closeStatusNormal            = 1000
	closeStatusGoingAway         = 1001
	closeStatusProtocolError     = 1002
	closeStatusUnsupportedData   = 1003
	closeStatusFrameTooLarge     = 1004
	closeStatusNoStatusRcvd      = 1005
	closeStatusAbnormalClosure   = 1006
	closeStatusBadMessageData    = 1007
	closeStatusPolicyViolation   = 1008
	closeStatusTooBigData        = 1009
	closeStatusExtensionMismatch = 1010
)

var (
	// Websocket协议规定的报头列表
	handshakeHeaders = map[string]bool{
		"Host":                   true,
		"Upgrade":                true,
		"Connection":             true,
		"Sec-Websocket-Key":      true,
		"Sec-Websocket-Origin":   true,
		"Sec-Websocket-Version":  true,
		"Sec-Websocket-Protocol": true,
		"Sec-Websocket-Accept":   true,
	}
)

type hybiFrameHeader struct {
	// 是否是最后一个消息片段
	Fin bool
	// RSV1, RSV2, RSV3, 用于自定义协议， 一般为0
	Rsv [3]bool
	// 操作类型
	OpCode byte
	// 载荷长度
	Length int64
	// 掩码键
	MaskingKey []byte

	// 报头原始数据
	data *bytes.Buffer
}

// rwc 是面向流的网络连接， 其实现了io.ReadWriteClose接口
// buf 是对rwc的缓冲式的读写接口
func newHybiServerConn(config *Config, buf *bufio.ReadWriter, rwc io.ReadWriteCloser, req *http.Request) *Conn {
	if buf == nil {
		br := bufio.NewReader(rwc)
		bw := bufio.NewWriter(rwc)
		buf = bufio.NewReadWriter(br, bw)
	}

	wsconn := &Conn{
		config:             config,
		request:            req,
		buf:                buf,
		rwc:                rwc,
		PayloadType:        TextFrame,
		defaultCloseStatus: closeStatusNormal,
		frameReaderFactory: hybiFrameReaderFactory{buf.Reader},
		// 客户端才需要Masking-key
		frameWriterFactory: hybiFrameWriterFactory{buf.Writer, req == nil},
	}
	wsconn.frameHandler = &hybiFrameHandler{conn: wsconn}
	return wsconn
}

// 实现frameHandler接口， 用于处理数据帧
type hybiFrameHandler struct {
	conn        *Conn
	payloadType byte
}

func (h *hybiFrameHandler) HandleFrame(frame frameReader) (r frameReader, err error) {
	// TODO check MaskingKey
	return frame, nil
}

func (h *hybiFrameHandler) WriteClose(status int) (err error) {
	return
}
