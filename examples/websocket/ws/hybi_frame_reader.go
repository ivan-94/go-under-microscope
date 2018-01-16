package ws

// 这个文件实现了websocket帧解析/读取器

import (
	"bufio"
	"bytes"
	"io"
)

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
