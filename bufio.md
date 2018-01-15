# bufio

bufio 实现了待缓冲的 I/O. 它封装 io.Reader 或 io.Writer 接口对象， 创建另一个实现了该接口的对象， 这个实现提供了缓冲和一些文本 I/O 帮助函数

<!-- TOC -->

- [bufio](#bufio)
  - [Reader](#reader)
    - [数据结构](#数据结构)
    - [构造](#构造)
    - [Read](#read)
    - [Peek](#peek)
    - [ReadSlice](#readslice)
    - [ReadBytes](#readbytes)
  - [Writer](#writer)
    - [数据结构](#数据结构-1)
    - [Write](#write)
    - [Flush](#flush)

<!-- /TOC -->

## Reader

### 数据结构

```go
type Reader struct {
	buf          []byte    // buffer 缓冲区
	rd           io.Reader // 底层reader
	r, w         int       // buf的读和写偏移
	err          error     // 当前错误
	lastByte     int
	lastRuneSize int
}
```

### 构造

默认的缓冲区大小是 4096，可以通过 NewReaderSize 指定大小

### Read

实现 io.Reader, 这个方法一次最多会调用底层 Reader 一次， 因此返回值 n 可能会小于 len(p)

```go
func (b *Reader) Read(p []byte) (n int, err error) {
  // p 为空
	n = len(p)
	if n == 0 {
		return 0, b.readErr()
  }
  // 表示读和写偏移在同一个位置， buffer为空
  // 这时候需要从底层的reader中读取数据到buffer中
	if b.r == b.w {
		if b.err != nil {
			return 0, b.readErr()
    }
    // p大于缓冲区， 这时候缓冲区没什么意义，可以直接读取到p中, 避免多余的复制
		if len(p) >= len(b.buf) {
			n, b.err = b.rd.Read(p)
			if n < 0 {
				panic(errNegativeRead)
			}
			if n > 0 {
				b.lastByte = int(p[n-1])
				b.lastRuneSize = -1
			}
			return n, b.readErr()
    }

    // 读取底层Reader到buffer
    // 不要使用fill, fill可能会导致循环
		b.r = 0
		b.w = 0
		n, b.err = b.rd.Read(b.buf)
		if n < 0 {
			panic(errNegativeRead)
		}
		if n == 0 {
			return 0, b.readErr()
		}
		b.w += n
	}

  // 直接从buffer中读取
	n = copy(p, b.buf[b.r:b.w])
	b.r += n
	b.lastByte = int(b.buf[b.r-1])
	b.lastRuneSize = -1
	return n, nil
}
```

### Peek

Peek 读取输入流的下 n 个字节，而不会移动读取的偏移。n 不能比缓冲区大

```go
func (b *Reader) Peek(n int) ([]byte, error) {
	if n < 0 {
		return nil, ErrNegativeCount
	}

  // 缓冲数据小于n，以及多余缓冲区容量
	for b.w-b.r < n && b.w-b.r < len(b.buf) && b.err == nil {
    // 填充和重组缓冲区
		b.fill() // b.w-b.r < len(b.buf) => buffer is not full
	}

  // n 超出buff
	if n > len(b.buf) {
		return b.buf[b.r:b.w], ErrBufferFull
	}

	// 0 <= n <= len(b.buf)
	var err error
	if avail := b.w - b.r; avail < n {
		// buffer中没有足够的数据
		n = avail
		err = b.readErr()
		if err == nil {
			err = ErrBufferFull
		}
	}
	return b.buf[b.r : b.r+n], err
}
```

填充和重组缓冲区, fill 会尝试读取底层 Reader maxConsecutiveEmptyReads 次,
一旦读取到值就返回

```go
func (b *Reader) fill() {
	// reslice, 利用buffer前置(b.r之前)空闲空间
	if b.r > 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

  // buffer空间已满, 不需要填充
	if b.w >= len(b.buf) {
		panic("bufio: tried to fill full buffer")
	}

  // 最多允许maxConsecutiveEmptyReads次空读取, 一旦读取到数据就返回
  // 空读取一般会阻塞
	for i := maxConsecutiveEmptyReads; i > 0; i-- {
		n, err := b.rd.Read(b.buf[b.w:])
		if n < 0 {
			panic(errNegativeRead)
		}
		b.w += n
		if err != nil {
			b.err = err
			return
    }
    // 只读取一次
		if n > 0 {
			return
		}
	}
	b.err = io.ErrNoProgress
}
```

### ReadSlice

ReadSlice 会读取知道第一次遇到 delim 字节或遇到错误。 如果读取到 delim 之前缓冲区满了， ReadSlice 会失败并返回
ErrBufferFull

```go
func (b *Reader) ReadSlice(delim byte) (line []byte, err error) {
	for {
		// 在现有缓存中查找是否存在
		if i := bytes.IndexByte(b.buf[b.r:b.w], delim); i >= 0 {
			line = b.buf[b.r : b.r+i+1]
			b.r += i + 1
			break
		}

		// 存在未决(Pending)的错误, 直接返回
		if b.err != nil {
			line = b.buf[b.r:b.w]
			b.r = b.w
			err = b.readErr()
			break
		}

		// 缓存已满, 返回ErrBufferFull
		if b.Buffered() >= len(b.buf) {
			b.r = b.w
			line = b.buf
			err = ErrBufferFull
			break
		}

    // 填充缓存
		b.fill() // buffer is not full
	}
  //  ....
}
```

### ReadBytes

ReadBytes 会读取直到第一次遇到 delim 字节， 返回缓冲区中已读取的数据和 delim 字节切片.
ReadBytes 会尝试读取直到遇到 EOF, 这一点是和 ReadSlice 的主要区别

```go
func (b *Reader) ReadBytes(delim byte) ([]byte, error) {
	var frag []byte      // 当前片段
	var full [][]byte    // 存放所有片段
	var err error
	for {
		var e error
		frag, e = b.ReadSlice(delim)
    if e == nil { // 找到
			break
    }

		if e != ErrBufferFull { // 预期之外的异常
			err = e
			break
		}

		// 拷贝当前buffer， 并添加到full
		buf := make([]byte, len(frag))
		copy(buf, frag)
    full = append(full, buf)

    // 循环继续读取, 直到找到或遇到除ErrBufferFull之外的异常
	}

	// 分配一个buffer， 用于存储所有片段
	n := 0
	for i := range full {
		n += len(full[i])
	}
	n += len(frag)
  buf := make([]byte, n)

	// 复制所有片段
	n = 0
	for i := range full {
		n += copy(buf[n:], full[i])
	}
	copy(buf[n:], frag)
	return buf, err
}
```

## Writer

### 数据结构

```go
type Writer struct {
  err error
  buf []byte     // 缓冲区
  n int          // 写入偏移
  wr io.Writer   // 底层Writer
}
```

### Write

将内容写入缓冲区

```go
func (b *Writer) Write(p []byte) (nn int, err error) {
  // 写入的内容大于缓冲区的可用空间
	for len(p) > b.Available() && b.err == nil {
		var n int
		if b.Buffered() == 0 {
      // 缓冲区内没有缓冲数据， 直接写入， 避免复制
			n, b.err = b.wr.Write(p)
		} else {
      // 缓冲区有数据，必须追加到buffer中， 保证有序
			n = copy(b.buf[b.n:], p)
      b.n += n
      // 刷入底层writer, 以腾出空间
			b.Flush()
		}
		nn += n
		p = p[n:]
  }

	if b.err != nil {
		return nn, b.err
  }
  // 直接写入缓冲区
	n := copy(b.buf[b.n:], p)
	b.n += n
	nn += n
	return nn, nil
}
```

### Flush

将缓冲区的数据写入底层 Writer 接口

```go
func (b *Writer) Flush() error {
	if b.err != nil {
		return b.err
  }
  // 缓冲区无数据
	if b.n == 0 {
		return nil
  }
  // 写入底层数据
	n, err := b.wr.Write(b.buf[0:b.n])
	if n < b.n && err == nil {
		err = io.ErrShortWrite
  }

	if err != nil {
		if n > 0 && n < b.n {
      // 部分写入，重组缓冲区
			copy(b.buf[0:b.n-n], b.buf[n:b.n])
		}
		b.n -= n
		b.err = err
		return err
  }
  // 全部写入
	b.n = 0
	return nil
}
```
