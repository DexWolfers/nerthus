package queue

import (
	"container/list"
	"fmt"
	"sync"
	"testing"
	"time"
)

type message struct {
	tm time.Time
	b  []byte
}

func cmp(element *list.Element) bool {
	ok := time.Now().Sub(element.Value.(*message).tm) > time.Second*3
	if ok {
		fmt.Println("timeout", element.Value.(*message).tm)
	}
	return ok
}

/*
goos: darwin
goarch: amd64
pkg: gitee.com/nerthus/nerthus/common/queue
BenchmarkNewListQueue-4   	 3000000	       367 ns/op
PASS
*/
func BenchmarkNewListQueue(b *testing.B) {
	var listQ = NewListQueue(cmp)
	var g sync.WaitGroup
	g.Add(1)
	go func() {
		defer g.Done()
		for i := 0; i < b.N; i++ {
			listQ.PushBack(&message{tm: time.Now(), b: []byte{byte(i)}})
		}
	}()

	for i := 0; i < b.N+1; i++ {
		listQ.Pop()
	}

	g.Wait()
	for {
		iter := listQ.Pop()
		if iter == nil {
			break
		}
		//fmt.Println(iter.Value.(*message).tm)
	}
}
