package ws

// 这个文件实现了websocket 数据帧的封装和写入

import "bufio"

// 用于创建一个帧写入器
type hybiFrameWriterFactory struct {
	*bufio.Writer
	needMaskingKey bool
}
