package ws

// 这个文件实现了websocket的握手过程

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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

// 接受websocket
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
