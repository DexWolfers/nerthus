package tracetime

import (
	"strings"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/log"
)

func TestTraceTime_Step(t *testing.T) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Ctx...)
		return nil
	}))
	//注意：不能随意调整代码行，因为测试时根据代码号处理
	trace := New(nil, 0)
	defer trace.Stop()
	trace.Tag() // [16-17]
	time.Sleep(time.Second)
	//empty
	trace.Tag() //[19-20]

	if len(trace.tags) != 2 {
		t.Fatalf("should be have two tag,but got %d", len(trace.tags))
	}
	tag1 := trace.tags[0]
	if tag1.beginLine != 17 {
		t.Fatalf("tag1 should be begin with line:17,got %d", tag1.beginLine)
	}
	if tag1.endLine != 18 {
		t.Fatalf("tag1 should be end with line:18,got %d", tag1.endLine)
	}
	tag2 := trace.tags[1]
	if tag2.beginLine != 20 {
		t.Fatalf("tag1 should be begin with line:20,got %d", tag2.beginLine)
	}
	if tag2.endLine != 21 {
		t.Fatalf("tag1 should be end with line:21,got %d", tag2.endLine)
	}
}

func TestTraceTime_Def(t *testing.T) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		t.Log(r.Ctx...)
		return nil
	}))

	defer New().SetMin(0).Stop()
}

func TestTraceTime_Msg(t *testing.T) {
	tr := New()
	tr.Tag("step1")
	tr.Tag("")
	tr.Tag()
	tr.Tag("step2")

	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		t.Log(r.Ctx...)
		if len(r.Ctx) != 3*2 {
			t.Fatal("values should be two items")
		}
		costInfo := r.Ctx[1]
		if !strings.Contains(costInfo.(string), "step1") {
			t.Fatalf("should be contains step1,in %s", costInfo)
		}
		if !strings.Contains(costInfo.(string), "step2") {
			t.Fatalf("should be contains step2,in %s", costInfo)
		}
		return nil
	}))

	tr.Stop()
}

// 只是用于查看日志格式
func TestTraceTime_print2(t *testing.T) {
	tr := New()
	tr.printToFile = true

	tr.Tag("step1")
	tr.Tag("")
	tr.Tag()
	tr.Tag("step2")

	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		t.Log(r.Ctx...)
		return nil
	}))

	tr.Stop()
}

func TestDure(t *testing.T) {
	tr := New()
	time.Sleep(time.Second)

	tr.printToFile = true
	now := time.Now().UnixNano()

	time.Sleep(time.Second * 3)
	tr.AddArgs("cost2", time.Since(time.Unix(0, int64(now))))
	tr.Tag()

	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		t.Log(r.Ctx...)
		return nil
	}))
	tr.Stop()
}

func TestTraceTime_Reset(t *testing.T) {
	tr := New()
	tr.printToFile = true
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		t.Log(r.Ctx...)
		return nil
	}))
	for i := 0; i < 10; i++ {
		tr.Reset("cost2", time.Since(time.Unix(0, int64(time.Now().UnixNano()))))
		time.Sleep(1 * time.Second)
		tr.Tag()
	}
}

func TestDelay(t *testing.T) {
	tr := New().SetMin(0)
	tr.printToFile = true
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		t.Log(r.Msg)
		for i := 0; i < len(r.Ctx); i += 2 {
			t.Logf("%s=%s", r.Ctx[i], r.Ctx[i+1])
		}
		return nil
	}))

	tr.AddArgs("str", "abc", "hash", common.StringToHash("hashvalue"))
	tr.AddArgs("cost2", delayF(func() interface{} {
		time.Sleep(2 * time.Second)
		return 100
	}))

	time.Sleep(time.Second)
	tr.Tag()
	tr.Stop()
}
