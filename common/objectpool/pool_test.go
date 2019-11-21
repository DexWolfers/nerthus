package objectpool

import (
	"fmt"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"
)

func TestObjPool(t *testing.T) {
	pl := NewObjPool(false, time.Second*10, func(s ObjectKey) Object {
		return &TestObj{
			name: s.Text(),
		}
	})
	pl.Start()
	for i := 0; i < 10; i++ {
		key := testKey(fmt.Sprintf("obj%d", i))
		pl.Handle(key, "hello", i)
	}
	go func() {
		for {
			key := testKey(fmt.Sprintf("obj%d", rand.Intn(10)))
			value := rand.Intn(1000)
			pl.Handle(key, "world", value)
			time.Sleep(time.Second)
		}
	}()
	//for i := 0; i < 10; i++ {
	//	pl.Free(fmt.Sprintf("obj%d", i), nil)
	//}
	<-time.After(time.Minute)
}

func TestUpper(t *testing.T) {
	go http.ListenAndServe("127.0.0.1:6060", nil)
	wg := sync.WaitGroup{}
	ch := make(chan string, 1024)
	pl := NewObjPool(false, time.Second*10, func(s ObjectKey) Object {
		return &TestObj{
			name: s.Text(),
			done: ch,
		}
	})
	pl.SetUpperLimit(10)
	pl.SetWaitUpper(30)
	pl.Start()
	wg.Add(20)
	checkFunc := func() {
		fmt.Println(pl.container.Count(), pl.waitQueue.Count(), pl.waitQueueCh.Len(), pl.freeChan.Count())
	}
	go func() {
		for {
			select {
			case e := <-ch:
				key := testKey(e)
				pl.Free(key, func(s string) {
					//fmt.Printf("free %s \n", s)
					wg.Done()
					checkFunc()
				})
			}
		}
	}()
	id := 0
	for {
		key := testKey(fmt.Sprintf("[%d]", id%20))
		pl.Handle(key, "hello", id%20)
		id++
		if id >= 400 {
			break
		}
	}
	checkFunc()
	wg.Wait()
	checkFunc()
}

type testKey string

func (t testKey) Text() string {
	return string(t)
}

type TestObj struct {
	name string
	seq  int
	done chan string
}

func (t *TestObj) Start() {
	//fmt.Println(">>>starting", t.name)
}
func (t *TestObj) Stop() {
	//fmt.Println(">>>stoping", t.name)

}
func (t *TestObj) Reset(key ObjectKey, args ...interface{}) {
	t.name = key.Text()
	t.seq = 0
}
func (t *TestObj) Handle(args []interface{}) {
	t.seq++
	//fmt.Printf("---%s--- seq=%d >%v\n", t.name, t.seq, args)
	if t.seq == 20 {
		//fmt.Printf("done %s\n", t.name)
		t.done <- t.name
	}
}
