package batch

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBatchExec_DefaultPush(t *testing.T) {
	Convey("PUSH", t, func() {
		be := NewBatchExec(1)
		be.Start()
		defer be.Stop()

		f := func(gp interface{}, args ...interface{}) {
			Convey("item", t, func() {
				if gp == "group1" {
					So(args, ShouldHaveLength, 3) //a1,b2,c3
					So(args[0], ShouldEqual, "a1")
					So(args[1], ShouldEqual, "b2")
					So(args[2], ShouldEqual, "c3")

				} else if gp == "group2" {
					So(args, ShouldHaveLength, 0)

					// 任务还在处理中，队列数保持不变
					So(be.chs[0].queuedCount, ShouldEqual, 1)

				} else {
					t.Fatalf("except group1 or group2, got %s", gp)
				}
			})
		}
		be.Push(f, "group1", "a1", "b2", "c3")
		be.Push(f, "group2")

		time.Sleep(time.Second)
		So(be.chs[0].queuedCount, ShouldEqual, 0)

		// 任务结束，不应该在Group存留
		_, ok := be.chs[0].queuedGroup.Load("group")
		So(ok, ShouldBeFalse)
	})
}

func TestBatchExec_Push(t *testing.T) {
	Convey("BatchPush", t, func() {
		var max = 10
		count := 10000
		be := NewBatchExec(max)
		be.testMode = true

		be.Start()
		defer be.Stop()

		var runLoc sync.Map
		lockCh := make(chan struct{})
		testGroup := sync.Map{} // make(map[string]interface{})

		var wg sync.WaitGroup
		f := func(gp interface{}, args ...interface{}) {
			// key = gp +index  value = channel index
			key := fmt.Sprintf("%s_%d", gp, args[1])
			runLoc.Store(key, args[0])
			//休息会儿
			time.Sleep(time.Nanosecond * time.Duration(args[1].(int)))

			//解锁
			if args[1].(int) == count-1 {
				close(lockCh) //最后一个任务处理时，关闭
			} else if _, ok := testGroup.Load(gp); ok { // 默认进行阻塞
				<-lockCh
			}

			wg.Done()
		}

		for i := 0; i < count; i++ {
			wg.Add(1)
			gp := fmt.Sprintf("group_%d", i%10)
			// 将第3，6，9组进行阻塞测试，结果是该组只会出现在一个CHANEL中
			if g := (i % 10); g == 1 || g == 5 || g == 8 {
				testGroup.Store(gp, -1)
			}
			go be.Push(f, gp, i)
		}
		wg.Wait()

		workChs := make(map[interface{}]int)
		for i := 0; i < count; i++ {
			gp := fmt.Sprintf("group_%d", i%10)
			key := fmt.Sprintf("%s_%d", gp, i)
			chIndex, _ := runLoc.Load(key)
			workChs[chIndex]++

			// 检查测试组
			if v, ok := testGroup.Load(gp); ok {
				if v == -1 {
					testGroup.Store(gp, chIndex) //记录第一次出现的Chain Index
				} else {
					So(v, ShouldEqual, chIndex) //后续则必须同样出现在相同的Chain Index 中
				}
			}
		}
		// 所有Chain 应该均有分配任务
		So(workChs, ShouldHaveLength, max)

	})
}

func BenchmarkExecChannel_Add(b *testing.B) {
	be := NewBatchExec(100)
	be.testMode = true
	defer be.Stop()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var gp int

		var wg sync.WaitGroup
		for pb.Next() {
			gp++

			wg.Add(1)
			be.Push(func(gp interface{}, args ...interface{}) {
				wg.Done()
			}, strconv.Itoa(gp%2000), gp)
		}
		wg.Wait()
	})
}
