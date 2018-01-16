package ws

// 这个文件实现了hybi草案的协议

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"io/ioutil"
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

	// 控制帧(Control Frames) 包括Close, Ping, Pong. 控制帧的载荷最大长度不会超过125
	maxControlFramePayloadLength = 125
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
	if h.conn.IsServerConn() {
		// 客户端请求必须带maskingkey
		if frame.(*hybiFrameReader).header.MaskingKey == nil {
			h.WriteClose(closeStatusProtocolError)
			return nil, io.EOF
		}
	} else {
		// 服务端必须没有mask所有帧
		if frame.(*hybiFrameReader).header.MaskingKey != nil {
			h.WriteClose(closeStatusProtocolError)
			return nil, io.EOF
		}
	}

	// 这一步有什么用? 清空header?
	if header := frame.HeaderReader(); header != nil {
		io.Copy(ioutil.Discard, header)
	}

	switch frame.PayloadType() {
	// 分片
	case ContinuationFrame:
		frame.(*hybiFrameReader).header.OpCode = h.payloadType
	case TextFrame, BinaryFrame:
		h.payloadType = frame.PayloadType()
	case CloseFrame:
		return nil, io.EOF
	case PingFrame, PongFrame:
		b := make([]byte, maxControlFramePayloadLength)
		n, err := io.ReadFull(frame, b)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		// 忽略剩余的数据
		io.Copy(ioutil.Discard, frame)
		// 回复Pong
		if frame.PayloadType() == PingFrame {
			if _, err := h.WritePong(b[:n]); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	return frame, nil
}

func (h *hybiFrameHandler) WriteClose(status int) (err error) {
	h.conn.wio.Lock()
	defer h.conn.wio.Unlock()
	w, err := h.conn.frameWriterFactory.NewFrameWriter(CloseFrame)
	if err != nil {
		return err
	}
	msg := make([]byte, 2)
	// 载荷的前两个字节必须是无符号的整数(以网络字节序)
	// 后续可选内容是utf-8编码的数据, 一般用于调试
	binary.BigEndian.PutUint16(msg, uint16(status))
	_, err = w.Write(msg)
	w.Close()
	return err
}

func (h *hybiFrameHandler) WritePong(msg []byte) (n int, err error) {
	h.conn.wio.Lock()
	defer h.conn.wio.Unlock()
	w, err := h.conn.frameWriterFactory.NewFrameWriter(PongFrame)
	if err != nil {
		return 0, err
	}
	n, err = w.Write(msg)
	w.Close()
	return n, err
}
