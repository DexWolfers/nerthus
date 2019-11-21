package chancluster

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/crypto"
	"github.com/stretchr/testify/require"
)

func TestChanController(t *testing.T) {
	quit := make(chan struct{})
	sequence := int64(0)
	waitQueue := make(chan struct{})
	go func() {
		for index := 1; index <= 100; index++ {
			n := fmt.Sprintf("con%d", index)
			cotr := &channelController{
				n,
				NewDychan(10),
				func(ev []interface{}) error {
					fmt.Printf("%v -> %v\n", n, ev)
					return nil
				},
				nil,
				1,
				true,
				time.Second * 10,
				quit,
				&sequence,
				func(i *channelController) {
					fmt.Printf("---------%v channel controller stop-------\n", i.name)
				},
				waitQueue,
			}
			cotr.start(quit)
			go func() {
				times := 1000
				for {
					cotr.put([]interface{}{"hello world", times, time.Now()})
					time.Sleep(time.Millisecond)
					times--
					if times <= 0 {
						break
					}
				}
				fmt.Println("send finished ", cotr.name)
			}()
			time.Sleep(time.Second)
		}
	}()
	<-time.After(time.Second * 20)
	close(quit)
	<-time.After(time.Minute)
}

func TestChanCluster_Handle(t *testing.T) {
	q := NewChanCluster()

	var wg sync.WaitGroup
	wg.Add(1)
	q.Register(struct{}{}, 1, 20, func(i []interface{}) error {
		wg.Done()
		return nil
	}, nil)
	q.Start()
	defer q.Stop()

	err := q.Handle(struct{}{}, "good")
	require.NoError(t, err)

	wg.Wait()
}

func TestChanCluster_Handle2(t *testing.T) {
	q := NewChanCluster()

	gs, ids := 10, 6
	var wg sync.WaitGroup
	wg.Add(gs * ids)

	var count int64
	q.testOnNew = func(identify interface{}) {
		atomic.AddInt64(&count, 1)
		t.Log(identify)
	}

	for i := 0; i < gs; i++ {
		go func() {
			for i := 0; i < ids; i++ {
				q.HandleOrRegister(i, 1, 500, func(i []interface{}) error {
					wg.Done()
					return nil
				}, nil)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, ids, int(count), "should only create %d gorountimes", ids)
}

func BenchmarkChanCluster_Handle(b *testing.B) {

	run := func(gs int, b *testing.B) {
		q := NewChanCluster()
		q.Start()
		defer q.Stop()

		var wg sync.WaitGroup
		wg.Add(b.N)

		msg, _ := hex.DecodeString("ce0677bb30baa8cf067c88db9811f4333d131bf8bcf12fe7065d211dce971008")
		sig, _ := hex.DecodeString("90f27b8b488db00b00606796d2987f6a5f59ae62ea05effe84fef5b8b0e549984a691139ad57a3f0b906637673aa2f63d1f55cb1a69199d4009eea23ceaddc9301")

		handle := func(i []interface{}) error {
			crypto.Ecrecover(msg, sig)
			wg.Done()
			return nil
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q.HandleOrRegister(i/gs, 1, 5000, handle, nil)
		}
		wg.Wait()
	}

	for i := 1; i < 50; i++ {
		gs := i * 200
		b.Run("g"+strconv.Itoa(gs), func(b *testing.B) {
			run(gs, b)
		})
	}

}

func TestUpperLimit(t *testing.T) {
	clt := NewChanCluster()
	clt.SetUpperLimit(100)
	clt.Start()
	wg := sync.WaitGroup{}
	for i := 0; i < 200; i++ {
		wg.Add(1)
		clt.HandleOrRegister(i, 1, 100, func(i []interface{}) error {
			time.Sleep(time.Second * 3)
			wg.Done()
			fmt.Printf("---------%v handle deal----------\n", i)
			return nil
		}, nil, i)
		fmt.Printf(">>>>>>>>>%v register----------\n", i)
	}
	wg.Wait()
}
func TestFree(t *testing.T) {
	clt := NewChanCluster()
	clt.Start()
	func1 := func() {
		for id := 0; id < 100; id++ {
			clt.HandleOrRegister(id, 1, 128, func(i []interface{}) error {
				fmt.Printf("-----------%v handle deal\n", i)
				return nil
			}, nil, id)
		}
	}
	timeout := time.After(time.Second * 1)
lp:
	for {
		select {
		case <-timeout:
			break lp
		default:
			func1()
		}
	}
	time.Sleep(time.Second * 12)
	func1()
	time.Sleep(time.Minute)
}
