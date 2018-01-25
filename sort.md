# sort

sort 包实现了切片和自定义容器的排序原语. 通过这个包可以学习 Go 接口的典型用法和编程思维.

## Interface

只有容器实现了 Interface 接口, 就可以通过 sort 包进行排序

```go
type Interface interface {
  // 返回容器的大小
  Len() int
  // 给定一个索引, 用于比较, i是否小于j
  Less(i, j int) bool
  // 互换位置
	Swap(i, j int)
}
```

## Reverse

简单实现反向排序, 只需调换 Interface 的 Less 的 i, j 索引即可

```go
type reverse struct {
	Interface
}
// 调换源Interface的Less方法的索引
func (r reverse) Less(i, j int) bool {
	return r.Interface.Less(j, i)
}

func Reverse(data Interface) Interface {
	return &reverse{data}
}
```

## Sort 方法

sort 方法实现很简单, 接收一个 Interface 接口数据, 调用 quickSort 算法进行排序, 具体的排序算法不在讨论范围之内.

```go
func Sort(data Interface) {
	n := data.Len()
	quickSort(data, 0, n, maxDepth(n))
}
```

## Interface 实现

sort 包中, 包含了常见类型的 Interface 实现, 类型命名为<Type>Slice, 如 IntSlice, StringSlice, Float64Slice;
除此之外, 还有一些快捷排序方法, 方法命名为<Type>s(Type[]), 如 Ints

```go
type IntSlice []int

// 实现Interface接口
func (p IntSlice) Len() int           { return len(p) }
func (p IntSlice) Less(i, j int) bool { return p[i] < p[j] }
func (p IntSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// 快捷排序方法
func (p IntSlice) Sort() { Sort(p) }

// 快捷方法
func Ints(a []int) { Sort(IntSlice(a)) }
```

使用示例:

```go
a := []int{8, 2, 1, 4, 9}
sort.Sort(sort.IntSlice(a))

// 或
sort.IntSlice(a).Sort()
```
