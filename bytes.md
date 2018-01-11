# bytes

bytes 包实现了 byte 切片的操作方法，方法集合类似于 strings

<!-- TOC -->

- [bytes](#bytes)
  - [Buffer](#buffer)
    - [数据结构](#数据结构)
    - [方法](#方法)
  - [Reader](#reader)

<!-- /TOC -->

## Buffer

buffer 是一个变长的字节缓存序列。实现了写入和读取方法.

### 数据结构

```go
type Buffer struct {
	buf       []byte   // buffer的内容切片，可读内容是 buf[off:len(buf)]
	off       int      // 偏移， 读取从 &buf[off]开始, 写入从 &buf[len(buf)] 开始
	bootstrap [64]byte // 用于为小的buffer快速分配内存
	lastRead  readOp   // 最后的的操作类型， 主要用于判断能否回写(unread)
}
```

### 方法

**创建**
即设置初始 buf， 注意这里和原来的 buf 共享底层数组

```go
func NewBuffer(buf []byte) *Buffer { return &Buffer{buf: buf} }
```

**写入**
向 buffer 中写入缓存，并按需扩充容量

```go
func (b *Buffer) Write(p []byte) (n int, err error) {
  b.lastRead = opInvalid
	m, ok := b.tryGrowByReslice(len(p)) // 首先看buf是否有足够的容量, 如果有就重新slice
	if !ok {
		m = b.grow(len(p))                // 增长buffer
  }
  // m 是写入的数据的索引
	return copy(b.buf[m:], p), nil
}

func (b *Buffer) tryGrowByReslice(n int) (int, bool) {
  // 有足够的容量
	if l := len(b.buf); n <= cap(b.buf)-l {
    // 重新slice
		b.buf = b.buf[:l+n]
		return l, true
	}
	return 0, false
}
```

**增长 buffer**

```go
func (b *Buffer) grow(n int) int {
	m := b.Len() // buffer 目前的数据长度(len - off)
	// 如果数据长度为空， 且有偏移(说明有容量); 可以重新利用整个内存空间
	if m == 0 && b.off != 0 {
		b.Reset()
	}
	// 尝试重新reslice， 看是否有空间(后向空间)
	if i, ok := b.tryGrowByReslice(n); ok {
		return i
	}
	// 查看是否可以利用bootstrap内存空间
	if b.buf == nil && n <= len(b.bootstrap) {
		b.buf = b.bootstrap[:n]
		return 0
  }

	c := cap(b.buf)
	if n <= c/2-m {
    // 查看是否可以利用“前向空间”
    // 一般情况下，只要 m+n <= c即可, 但是这里设置为 m+n <= c/2
    // 这是为了减少复制的时间
		copy(b.buf, b.buf[b.off:])
	} else if c > maxInt-c-n {
    // 太大了， 不能再分配了, 即 2*c + n > maxInt
		panic(ErrTooLarge)
	} else {
    // 没有足够的空间，重新分配一个slice
		buf := makeSlice(2*c + n)
		copy(buf, b.buf[b.off:])
		b.buf = buf
	}
	//重新设置偏移和len
	b.off = 0
	b.buf = b.buf[:m+n]
	return m
}
```

**读取**
从 buffer 中读取指定长度字节序列，直到 buffer 为空(到达尾部)。

```go
func (b *Buffer) Read(p []byte) (n int, err error) {
	b.lastRead = opInvalid
	if b.empty() {
		// buffer 无内容可读
		// Buffer 为空，回收
		b.Reset()
		if len(p) == 0 {
			// p长度为空，无法写入
			return 0, nil
		}
		// 到达尾部
		return 0, io.EOF
	}
	// 复制到p，copy根据p或b的容量进行写入
	n = copy(p, b.buf[b.off:])
	// 更新偏移值
	b.off += n
	if n > 0 {
		b.lastRead = opRead
	}
	return n, nil
}
```

**回写**
将上次读取的字节回写到 buffer 中

```go
	// 上次操作不是有效读取读取
	if b.lastRead == opInvalid {
		return errors.New("bytes.Buffer: UnreadByte: previous operation was not a successful read")
	}
	b.lastRead = opInvalid
	if b.off > 0 {
		b.off--
	}
	return nil
```

## Reader

和 Buffer 类似，只不过 Reader 是只读的。 只有读取方法。此外 Reader 还支持 Seek
