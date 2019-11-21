package objectpool

// 可入池对象接口
type Object interface {
	Start()
	Stop()
	Reset(key ObjectKey, args ...interface{})
	Handle(args []interface{})
	Key() ObjectKey
}

// key接口
type ObjectKey interface {
	Text() string
}
