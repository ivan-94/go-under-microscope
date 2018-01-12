# io

io 包提供了 I/O 原语(primitives)的基本接口.

<!-- TOC -->

- [io](#io)
  - [Reader](#reader)
    - [特殊类型的 Reader](#特殊类型的-reader)
  - [Writer](#writer)
  - [Closer](#closer)
  - [Seeker](#seeker)
  - [Pipe](#pipe)
    - [管道实现](#管道实现)
      - [数据结构](#数据结构)
      - [读取方法](#读取方法)
  - [组合接口](#组合接口)
  - [其他](#其他)
- [](#)

<!-- /TOC -->

## Reader

Reader 接口定义了基本的读取方法.

```go
type Reader interface {
  // Read 方法读取len(p)长度的数据到p中, 返回读取的长度(0 <= n <= len(p))
  // 或者抛出错误, 可以查看源码详细注释或 ./bytes.md中Buffer对Reader的实现
	Read(p []byte) (n int, err error)
}
```

### 特殊类型的 Reader

* func LimitReader(r Reader, n int64) Reader
  从 r 中读取限定数量的数据, 到达到 n 时, 将返回 EOF
* func MultiReader(readers ...Reader) Reader
  从多个 reader 中读取数据, 按照顺序依次读取, 只有前一个到达 EOF 时, 才开始读取下一个 reader
* func TeeReader(r Reader, w Writer) Reader
  从 r 中读取数据, 并写入到 w 中
* func NewSectionReader(r ReaderAt, off int64, n int64) \*SectionReader
  类似于 LimitReader, 从 r 中读取限定片段

## Writer

Writer 接口定义基本的读取方法

```go
type Writer interface {
  // 向目标对象写入len(p)长度的数据, 返回读取的长度(0 <= n <= len(p))
  // 如果n < len(p), 必须返回非nil错误
  // 写入方法不能修改p
	Write(p []byte) (n int, err error)
}
```

## Closer

Closer 接口定义基本的关闭方法

```go
type Closer interface {
  // 具体行为依赖你的实现
	Close() error
}
```

## Seeker

定义了基本的 seek 方法, seek 方法可以设置下一次的读或写的偏移位置

```go
type Seeker interface {
  // io.SeekStart 表示相对于文件的开头
  // io.SeekCurrent 表示相对于当前偏移
  // io.SeekEnd 表示相对于文件结尾
  // 返回新的偏移值
	Seek(offset int64, whence int) (int64, error)
}
```

## Pipe

**双向管道**
Pipe 方法创建一个同步的内存管道, 可以用于连接 io.Reader 到 io.Writer. 管道是线程安全的.

```go
func Pipe() (*PipeReader, *PipeWriter)
```

**半开管道**

* PipeReader: 读取端, 支持 Close, CloseWithError, Read 等方法
* type PipeWriter: 写入端, 支持 Close, CloseWithError, Write 等方法

### 管道实现

#### 数据结构

```go
type pipe struct {
	wrMu sync.Mutex   // 用于序列化写入操作
	wrCh chan []byte  // 写入通道
	rdCh chan int     // 读取通道

	once sync.Once // 用于保护close操作, 即只能关闭一次
	done chan struct{}
	rerr atomicError
	werr atomicError
}
```

#### 读取方法

```go
func (p *pipe) Read(b []byte) (n int, err error) {
	select {
	case <-p.done:
		return 0, p.readCloseError()
	default:
	}

	select {
	case bw := <-p.wrCh:
		nr := copy(b, bw)
		p.rdCh <- nr
		return nr, nil
	case <-p.done:
		return 0, p.readCloseError()
	}
}
```

## 组合接口

io 中还定义许多组合接口

```go
type ReadWriter interface {
	Reader
	Writer
}
type ReadCloser interface {
	Reader
	Closer
}
type WriteCloser interface {
	Writer
	Closer
}
```

## 其他

io 中还定义了其他基本 I/O 行为接口, 如 ReadFrom, WriteTo, ReadAt 等, 以及基于这些接口实现的简单方法, 如 Copy, ReadAtLeast

##
