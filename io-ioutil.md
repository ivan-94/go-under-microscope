# io/ioutil

ioutil 包主要封装了 os 包下的一些文件操作方法

<!-- TOC -->

- [io/ioutil](#ioioutil)
  - [ReadFile](#readfile)

<!-- /TOC -->

## ReadFile

读取整个文件

```go
func ReadFile(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
  defer f.Close()
  // FileInfo是不能保证返回确切的文件大小的, 所以这里
  // 稍微提供一点冗余空间, 避免多余的内存分配
	var n int64 = bytes.MinRead

	if fi, err := f.Stat(); err == nil {
		if size := fi.Size() + bytes.MinRead; size > n {
			n = size
		}
	}
	return readAll(f, n)
}

func readAll(r io.Reader, capacity int64) (b []byte, err error) {
	var buf bytes.Buffer
	defer func() {
    // 从panic中恢复
		e := recover()
		if e == nil {
			return
		}
		if panicErr, ok := e.(error); ok && panicErr == bytes.ErrTooLarge {
      // 是否是buffer溢出
			err = panicErr
		} else {
			panic(e)
		}
  }()
  // 分配容量
	if int64(int(capacity)) == capacity {
		buf.Grow(int(capacity))
  }
  // 详见bytes/buffer
	_, err = buf.ReadFrom(r)
	return buf.Bytes(), err
}
```
