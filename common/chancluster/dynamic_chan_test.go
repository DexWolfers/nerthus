package chancluster

import (
	"fmt"
	"math/big"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"
)

func TestDychan(t *testing.T) {
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	ch := NewDychan(5024)
	quit := make(chan struct{})
	count := 10000
	go func(c int) {
		for {
			b := big.NewInt(int64(rand.Int()))
			ch.In(b)
			c--
			if c <= 0 {
				break
			}
			//time.Sleep(time.Second)
			//fmt.Println("rest", c)
		}
		close(quit)
	}(count)

loop:
	for {
		select {
		case m := <-ch.Out():
			fmt.Println(m)
			count--
			time.Sleep(time.Second)
		case <-quit:
			break loop
		}
	}
	fmt.Println("send finished")
	//time.Sleep(3 * time.Second)
loop2:
	for {
		select {
		case m := <-ch.Out():
			fmt.Println(m)
			count--
			if count <= 0 {
				break loop2
			}
		}
	}
	fmt.Println("receive finished")
}

func TestUpper(t *testing.T) {
	go http.ListenAndServe("127.0.0.1:6060", nil)
	wg := sync.WaitGroup{}
	dy := NewDychan(100)
	go func() {
		time.Sleep(time.Second)
		for {
			//fmt.Println(">>>>>>rec len", len(dy.Out()))
			<-dy.Out()
			//v := <-dy.Out()
			//fmt.Printf("----received %v\n", v)
			wg.Done()
		}
	}()
	go func() {
		for {
			time.Sleep(3 * time.Second)
			fmt.Printf("-----len seq: %v\n", dy.Len())
		}
	}()
	wg.Add(2000)
	for i := 0; i < 2000; i++ {
		dy.In(i)
		fmt.Printf("----inserted %v\n", i)
	}
	wg.Wait()
}
