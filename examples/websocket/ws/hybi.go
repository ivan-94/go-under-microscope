package ws

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// 这个文件实现了hybi草案的协议

// hybiServerHandshaker 负责服务端握手
type hybiServerHandshaker struct {
	*Config
	accept []byte
}

// 读取客户端握手数据
func (c *hybiServerHandshaker) ReadHandshake(buf *bufio.Reader, req *http.Request) (code int, err error) {
	c.Version = ProtocolVersionHybi13
	// websocket 必须有GET发起
	if req.Method != http.MethodGet {
		return http.StatusMethodNotAllowed, ErrBadRequestMethod
	}

	// 协议升级, 检查一下报头
	// Connection:Upgrade
	// Sec-WebSocket-Extensions:permessage-deflate; client_max_window_bits
	// Sec-WebSocket-Key:JG03fivJ7gPpO/QWSHzPeQ==
	// Sec-WebSocket-Version:13
	// Upgrade:websocket
	if strings.ToLower(req.Header.Get("Upgrade")) != "websocket" ||
		!strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") {
		return http.StatusBadRequest, ErrNotWebSocket
	}

	// 没有包含nonce
	key := req.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return http.StatusBadRequest, ErrMissKey
	}

	// 检查版本
	version := req.Header.Get("Sec-WebSocket-Version")
	if version != SupportedProtocolVersion {
		return http.StatusBadRequest, ErrBadWebSocketVersion
	}

	var scheme string
	if req.TLS != nil {
		scheme = "wss"
	} else {
		scheme = "ws"
	}

	c.Location, err = url.ParseRequestURI(scheme + "://" + req.Host + req.URL.RequestURI())
	if err != nil {
		return http.StatusBadRequest, err
	}

	// Sec-WebSocket-Protocol如果存在， 表示客户端希望交互的子协议
	protocol := strings.TrimSpace(req.Header.Get("Sec-WebSocket-Protocol"))
	if protocol != "" {
		protocols := strings.Split(protocol, ",")
		for _, val := range protocols {
			c.Protocol = append(c.Protocol, strings.TrimSpace(val))
		}
	}

	c.accept, err = getNonceAccept([]byte(key))
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return http.StatusSwitchingProtocols, nil
}

// 生成accept值
// base64(sha1(sec-websocket-key + 258EAFA5-E914-47DA-95CA-C5AB0DC85B11))
func getNonceAccept(key []byte) (accept []byte, err error) {
	h := sha1.New()
	if _, err = h.Write(key); err != nil {
		return
	}

	if _, err = h.Write([]byte(websocketGUID)); err != nil {
		return
	}

	sum := h.Sum(nil)
	// 获取encode结果所需要的长度
	len := base64.StdEncoding.EncodedLen(len(sum))
	accept = make([]byte, len)
	base64.StdEncoding.Encode(accept, sum)
	return
}

// 接收websocket
func (c *hybiServerHandshaker) AcceptHandshake(buf *bufio.Writer) (err error) {
	if len(c.Protocol) > 0 && len(c.Protocol) != 1 {
		// 服务端必须从子协议中选择一个，这个行为在Server中自定义
		return ErrBadWebSocketProtocol
	}
	code := http.StatusSwitchingProtocols
	fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
	buf.WriteString("Upgrade: websocket\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Sec-WebSocket-Accept: " + string(c.accept) + "\r\n")

	if len(c.Protocol) > 0 {
		buf.WriteString("Sec-WebSocket-Protocol: " + c.Protocol[0] + "\r\n")
	}

	// 发送自定义报头
	if c.Header != nil {
		err = c.Header.WriteSubset(buf, handshakeHeaders)
		if err != nil {
			return
		}
	}

	buf.WriteString("\r\n")
	return buf.Flush()
}

func (c *hybiServerHandshaker) NewServerConn(buf *bufio.ReadWriter, rwc io.ReadWriteCloser, req *http.Request) *Conn {
	return newHybiServerConn(c.Config, buf, rwc, req)
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
		// frameWriterFactory: hybiFrameWriterFactory{buf.Writer, req == nil}, // ?
	}
	wsconn.frameHandler = &hybiFrameHandler{conn: wsconn}
	return wsconn
}

// 用于创建一个帧读取器
type hybiFrameReaderFactory struct {
	*bufio.Reader
}

//  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
// +-+-+-+-+-------+-+-------------+-------------------------------+
// |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
// |I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
// |N|V|V|V|       |S|             |   (if payload len==126/127)   |
// | |1|2|3|       |K|             |                               |
// +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
// |     Extended payload length continued, if payload len == 127  |
// + - - - - - - - - - - - - - - - +-------------------------------+
// |                               |Masking-key, if MASK set to 1  |
// +-------------------------------+-------------------------------+
// | Masking-key (continued)       |          Payload Data         |
// +-------------------------------- - - - - - - - - - - - - - - - +
// :                     Payload Data continued ...                :
// + - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - +
// |                     Payload Data continued ...                |
// +---------------------------------------------------------------+
func (buf hybiFrameReaderFactory) NewFrameReader() (frame frameReader, err error) {
	hybiFrame := new(hybiFrameReader)
	frame = hybiFrame
	var header []byte
	var b byte
	// 读取第一个字节， 包含FIN/RSV1/RSV2/RSV3/OpCode(4bits)
	b, err = buf.ReadByte()
	if err != nil {
		return
	}
	header = append(header, b)
	hybiFrame.header.Fin = ((header[0] >> 7) & 0x1) != 0
	for i := 0; i < 3; i++ {
		j := uint(6 - i)
		hybiFrame.header.Rsv[i] = ((header[0] >> j) & 0x1) != 0
	}
	hybiFrame.header.OpCode = header[0] & 0xf

	// 读取第二个字节， 包含Mask/Payload len(7bits)
	b, err = buf.ReadByte()
	if err != nil {
		return
	}
	header = append(header, b)
	mask := (b & 0x80) != 0
	// 剩余的位
	b &= 0x7f
	lengthFields := 0
	switch {
	case b <= 125: // payload length 7bits
		hybiFrame.header.Length = int64(b)
	case b == 126: // payload length 7 + 16bit, 即随后2个字节用来表示传输速度
		lengthFields = 2
	case b == 126: // payload length 7 + 64bit
		lengthFields = 8
	}

	// 读取payload length
	for i := 0; i < lengthFields; i++ {
		b, err = buf.ReadByte()
		if err != nil {
			return
		}

		if lengthFields == 8 && i == 0 {
			// 当为64bit时，最高有效位(most significant bit) 必须为0
			b &= 0x7f
		}

		header = append(header, b)
		hybiFrame.header.Length = (hybiFrame.header.Length << 8) + int64(b)
	}

	// 读取Masking-Key(0-4byte), 只有在mask存在时存在
	if mask {
		for i := 0; i < 4; i++ {
			b, err = buf.ReadByte()
			if err != nil {
				return
			}
			header = append(header, b)
			hybiFrame.header.MaskingKey = append(hybiFrame.header.MaskingKey, b)
		}
	}

	// 载荷读取器
	hybiFrame.reader = io.LimitReader(buf.Reader, hybiFrame.header.Length)
	hybiFrame.header.data = bytes.NewBuffer(header)
	hybiFrame.length = len(header) + int(hybiFrame.header.Length)
	return
}

// frameReader接口的实现
type hybiFrameReader struct {
	// 载荷读取器
	reader io.Reader
	// 帧报头
	header hybiFrameHeader
	// 当前读取偏移(主要用于掩码计算)
	pos int64
	// 帧大小： 包含报头和载荷
	length int
}

func (r *hybiFrameReader) PayloadType() byte {
	return r.header.OpCode
}

func (r *hybiFrameReader) Read(msg []byte) (n int, err error) {
	n, err = r.reader.Read(msg)
	// 掩码计算
	// 第 i byte 数据 = orig-data[i] ^ (i % 4)
	if r.header.MaskingKey != nil {
		for i := 0; i < n; i++ {
			msg[i] = msg[i] ^ r.header.MaskingKey[r.pos%4]
			r.pos++
		}
	}
	return n, err
}

func (r *hybiFrameReader) HeaderReader() io.Reader {
	if r.header.data == nil {
		return nil
	}
	if r.header.data.Len() == 0 {
		return nil
	}
	return r.header.data
}

func (r *hybiFrameReader) Len() int {
	return r.length
}

func (r *hybiFrameReader) TrailerReader() io.Reader {
	return nil
}

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

// 用于创建一个帧写入器
type hybiFrameWriterFactory struct {
	*bufio.Writer
	needMaskingKey bool
}
