// Author: @ysqi

package ntsdb

type PutGetter interface {
	Putter
	Getter
}

type Putter interface {
	Put(key []byte, value []byte) error
}
type Getter interface {
	Get(key []byte) ([]byte, error)
}

type Deleter interface {
	Delete(key []byte) error
}

type Finder interface {
	//按前缀遍历查找匹配项，注意：不保证顺序
	Iterator(prefix []byte, call func(key, value []byte) bool) error
}

type Database interface {
	PutGetter
	Deleter
	Finder
	Close()
	NewBatch() Batch
}

type Batch interface {
	DB() Database
	Putter
	Deleter
	Write() error
	Discard()
}
