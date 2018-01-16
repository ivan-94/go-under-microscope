package ws

// 这个文件实现了服务端的主要接口

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// Server 代表Websocket服务端
type Server struct {
	// 配置
	Config
	// 处理器, 类似于http.Handler
	Handler
	// 自定义握手行为，比如检查origin，选择子协议
	Handshake HandShaker
}

// 伺服Websocket
func (s Server) serveWebSocket(w http.ResponseWriter, req *http.Request) {
	// 是否实现了http.Hijacker
	hj, ok := w.(http.Hijacker)
	if !ok {
		panic("websocket serve failed: doesn't support Hijack")
	}
	// 对http连接进行劫持， 以接管接下来的TCP请求
	conn, rw, err := hj.Hijack()
	if err != nil {
		panic("websocket hijack failed")
	}

	// 如果客户端的握手不符合协议，将关闭连接
	defer conn.Close()
	// 新建Websocket服务连接, 主要进行握手，初始化配置和连接
	wsConn, err := newServerConn(conn, rw, req, &s.Config, s.Handshake)
	if err != nil {
		return
	}

	if wsConn == nil {
		panic("unexpected nil conn")
	}
	// 开始处理连接
	s.Handler(wsConn)
}

func newServerConn(conn net.Conn, buf *bufio.ReadWriter, req *http.Request, config *Config, handshake HandShaker) (wsConn *Conn, err error) {
	hs := hybiServerHandshaker{Config: config}
	code, err := hs.ReadHandshake(buf.Reader, req)
	if err == ErrBadRequestMethod {
		fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
		fmt.Fprintf(buf, "Sec-Websocket-Version: %s\r\n", SupportedProtocolVersion)
		buf.WriteString("\r\n")
		buf.WriteString(err.Error())
		buf.Flush()
		return
	}

	if err != nil {
		fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
		buf.WriteString("\r\n")
		buf.WriteString(err.Error())
		buf.Flush()
		return
	}

	// 自定义握手
	if handshake != nil {
		err = handshake(config, req)
		if err != nil {
			code = http.StatusForbidden
			fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
			buf.WriteString("\r\n")
			buf.Flush()
			return
		}
	}

	err = hs.AcceptHandshake(buf.Writer)
	if err != nil {
		code = http.StatusBadRequest
		fmt.Fprintf(buf, "HTTP/1.1 %03d %s\r\n", code, http.StatusText(code))
		buf.WriteString("\r\n")
		buf.Flush()
		return
	}

	// 新建一个Websocket连接
	wsConn = hs.NewServerConn(buf, conn, req)
	return
}

// HandShaker 用于自定义握手行为
type HandShaker func(*Config, *http.Request) error

// Handler Websocket 连接处理器, 实现了http.Handler, 用法类似于http.HandlerFunc
type Handler func(*Conn)

func (h Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s := Server{Handler: h}
	s.serveWebSocket(w, req)
}
