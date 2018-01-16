package ws

// 协议原文: https://tools.ietf.org/html/rfc6455
// 协议(中文解析)： http://blog.csdn.net/stoneson/article/details/8073285

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"sync"
)

const (
	// websocket支持版本
	ProtocolVersionHybi13    = 13
	ProtocolVersionHybi      = ProtocolVersionHybi13
	SupportedProtocolVersion = "13"

	// Websocket frame type
	// *  %x0 denotes a continuation frame
	// *  %x1 denotes a text frame
	// *  %x2 denotes a binary frame
	// *  %x3-7 are reserved for further non-control frames
	// *  %x8 denotes a connection close
	// *  %x9 denotes a ping
	// *  %xA denotes a pong
	// *  %xB-F are reserved for further control frames
	ContinuationFrame = 0
	TextFrame         = 1
	BinaryFrame       = 2
	CloseFrame        = 8
	PingFrame         = 9
	PongFrame         = 10
	UnknownFrame      = 255

	// 默认最大载荷 32MB
	DefaultMaxPayloadBytes = 32 << 20
)

var (
	// 请求方法错误
	ErrBadRequestMethod = &ProtocolError{"bad method"}
	// 不是Websocket请求
	ErrNotWebSocket        = &ProtocolError{"not websocket"}
	ErrMissKey             = &ProtocolError{"missing key"}
	ErrBadWebSocketVersion = &ProtocolError{"bad websocket verison"}
	// ErrBadWebSocketProtocol 表示服务端必须从子协议中选一个协议
	ErrBadWebSocketProtocol = &ProtocolError{"bad websocket Protocol"}
)

// ProtocolError 代表协议错误
type ProtocolError struct {
	ErrorString string
}

func (pe *ProtocolError) Error() string {
	return pe.ErrorString
}

// Config websocket 配置
type Config struct {
	// websocket协议版本
	Version int
	// websocket 服务地址
	Location *url.URL
	// 子协议
	Protocol []string
	// 额外的http报头，将在握手时一同发送
	Header http.Header
}

// Conn 是websocket 连接实现
type Conn struct {
	config  *Config
	request *http.Request

	buf *bufio.ReadWriter
	rwc io.ReadWriteCloser

	// 用于保护frameReader
	rio sync.Mutex
	// 帧读取器
	frameReaderFactory
	frameReader

	// 用于保护frameWriter
	wio sync.Mutex
	// 帧写入器
	frameWriterFactory
	frameWriter

	// 载荷类型
	PayloadType byte
	// 默认关闭状态
	defaultCloseStatus int
	MaxPayloadBytes    int

	frameHandler
}

// 服务端
func (c *Conn) IsServerConn() bool { return c.request != nil }

// 客户端
func (c *Conn) IsClientConn() bool { return c.request == nil }

func (c *Conn) Read(msg []byte) (n int, err error) {
	c.rio.Lock()
	defer c.rio.Unlock()
again:
	if c.frameReader == nil {
		// 新建reader
		frame, err := c.frameReaderFactory.NewFrameReader()
		if err != nil {
			return 0, err
		}
		c.frameReader, err = c.frameHandler.HandleFrame(frame)
		if err != nil {
			return 0, err
		}
		if c.frameReader == nil {
			goto again
		}
	}
	n, err = c.frameReader.Read(msg)
	return n, err
}

// frameReaderFactory 接口定义了创建帧读取器方法
type frameReaderFactory interface {
	NewFrameReader() (r frameReader, err error)
}

type frameReader interface {
	// Reader is to read payload of the frame.
	io.Reader

	// PayloadType returns payload type.
	PayloadType() byte

	// HeaderReader returns a reader to read header of the frame.
	HeaderReader() io.Reader

	// TrailerReader returns a reader to read trailer of the frame.
	// If it returns nil, there is no trailer in the frame.
	TrailerReader() io.Reader

	// Len returns total length of the frame, including header and trailer.
	Len() int
}

// frameWriterFactory 接口定义创建帧写入器方法
type frameWriterFactory interface {
	NewFrameWriter(payloadType byte) (w frameWriter, err error)
}

// 帧载荷写入器
type frameWriter interface {
	io.WriteCloser
}

// frameHandler 为处理数据帧定义了接口
type frameHandler interface {
	HandleFrame(frame frameReader) (r frameReader, err error)
	WriteClose(status int) (err error)
}
