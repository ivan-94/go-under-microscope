package ws

// 这个文件实现了websocket 数据帧的封装和写入

import (
	"bufio"
	"crypto/rand"
	"io"
)

// 用于创建一个帧写入器
type hybiFrameWriterFactory struct {
	*bufio.Writer
	needMaskingKey bool
}

func (fac hybiFrameWriterFactory) NewFrameWriter(payloadType byte) (w frameWriter, err error) {
	header := &hybiFrameHeader{Fin: true, OpCode: payloadType}
	if fac.needMaskingKey {
		// 生成Masking-Key
		header.MaskingKey, err = generateMaskingKey()
		if err != nil {
			return nil, err
		}
	}
	return &hybiFrameWriter{fac.Writer, header}, nil
}

type hybiFrameWriter struct {
	buf    *bufio.Writer
	header *hybiFrameHeader
}

// 构造websocket数据帧, 并写入
func (w *hybiFrameWriter) Write(msg []byte) (n int, err error) {
	var header []byte
	var b byte
	// Fin
	if w.header.Fin {
		b = 1 << 7
	}

	// RSV*
	for i := 0; i < 3; i++ {
		if w.header.Rsv[i] {
			b |= 1 << uint(6-i)
		}
	}

	// OpCode
	b |= w.header.OpCode
	header = append(header, b)

	// Mask
	if w.header.MaskingKey != nil {
		b = 1 << 7
	} else {
		b = 0
	}

	// payload length
	lengthField := 0
	length := len(msg)
	switch {
	case length <= 125:
		b |= byte(length)
	case length < 65536:
		b |= 126
		lengthField = 2 // + 16bit
	default:
		b |= 127
		lengthField = 8 // + 64bit
	}
	header = append(header, b)

	// extention payload length
	for i := 0; i < lengthField; i++ {
		j := uint((lengthField - i - 1) * 8)
		b = byte((length >> j) & 0xff)
		header = append(header, b)
	}

	// MaskingKey
	if w.header.MaskingKey != nil {
		if len(w.header.MaskingKey) != 4 {
			return 0, ErrBadMaskingKey
		}
		header = append(header, w.header.MaskingKey...)

		// 写入头部
		w.buf.Write(header)
		// 掩码
		data := make([]byte, length)
		for i := range data {
			data[i] = msg[i] ^ w.header.MaskingKey[i%4]
		}
		w.buf.Write(data)
		err = w.buf.Flush()
		return length, err
	}

	w.buf.Write(header)
	w.buf.Write(msg)
	err = w.buf.Flush()
	return length, err
}

func (w *hybiFrameWriter) Close() error {
	return nil
}

// generateMaskingKey 生成4个字节的随机字符串
func generateMaskingKey() (maskingKey []byte, err error) {
	maskingKey = make([]byte, 4)
	_, err = io.ReadFull(rand.Reader, maskingKey)
	return
}
