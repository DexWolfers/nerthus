package tracetime

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/log"
)

var Enable = false //是否启用跟踪，默认不开启。如果需要测试耗时跟踪，则需要开启，只用于测试环境测试

var enableFuns = map[string]struct{}{
	//"core.(*DagChain).writeRightUnit": {},
	//"core.(*DagChain).goodUnitLookup": {},
	"core.(*DagChain).writeRightUnit":              {},
	"core.(*DagChain).VerifyUnit":                  {},
	"core.(*TxPool).listenReceivedNewUnit":         {},
	"witness.(*BftMiningAss).generateUnit":         {},
	"consensus/bft/backend.(*backend).Commit":      {},
	"consensus/bft/core.(*core).handlePreprepare":  {},
	"consensus/bft/core.(*core).startNewRound":     {},
	"consensus/bft/core.(*core).handleRoundChange": {},
}

const projectHome = "gitee.com/nerthus/nerthus/"

type tagInfo struct {
	beginLine, endLine, tagLine int
	cost                        mclock.AbsTime
	msg                         string
}

// 调试日志，跟踪痕迹、耗时
type TraceTime struct {
	args       []interface{}
	functionID string
	logger     log.Logger

	start time.Time //开始时间
	end   time.Time //结束时间

	minCost       mclock.AbsTime //最小时间
	beginT, stepT mclock.AbsTime
	traceCost     mclock.AbsTime //用在Trace内部的时间开销
	curTag        int            //当前Tag位置
	tags          []tagInfo

	printToFile bool //是否是输出到文件

	enable bool
}

var emptyTrace = new(TraceTime)

func New(args ...interface{}) *TraceTime {
	if !Enable {
		return emptyTrace
	}

	now, start := mclock.Now(), time.Now()

	pc := make([]uintptr, 15)
	runtime.Callers(2, pc)
	f := runtime.FuncForPC(pc[0])
	functionID := f.Name()
	// 为了方便外部跟踪调用位置，直接打全路径，当然不需要显示项目路径
	functionID = strings.TrimPrefix(functionID, projectHome)
	if len(enableFuns) > 0 {
		if _, ok := enableFuns[functionID]; !ok {
			return emptyTrace
		}
	}

	_, _, line, _ := runtime.Caller(1)
	t := TraceTime{
		start:       start,
		beginT:      now,
		functionID:  functionID,
		stepT:       now,
		curTag:      line,
		logger:      log.Root(),
		printToFile: true, //测试情况下设置为是
		minCost:     mclock.AbsTime(time.Millisecond * 2),
		enable:      true,
	}
	t.traceCost += mclock.Now() - now //更新耗时
	t.AddArgs(args...)
	return &t
}

func (t *TraceTime) SetLog(logger log.Logger) *TraceTime {
	if !Enable || !t.enable {
		return t
	}
	t.logger = logger
	return t
}
func (t *TraceTime) SetMin(min time.Duration) *TraceTime {
	if !Enable || !t.enable {
		return t
	}
	t.minCost = mclock.AbsTime(min)
	return t
}

func (t *TraceTime) Tag(msg ...interface{}) *TraceTime {
	if !Enable || !t.enable {
		return t
	}
	t.tag(2, msg...)
	return t
}

func (t *TraceTime) AddArgs(args ...interface{}) *TraceTime {
	if !Enable || !t.enable {
		return t
	}
	for i := 0; i < len(args); i += 2 {
		// 封装依次Value，方便String
		t.args = append(t.args, args[i], myValue{args[i+1]})
	}
	return t
}

func (t *TraceTime) tag(skip int, msg ...interface{}) {
	if !Enable || !t.enable {
		return
	}
	now := mclock.Now()
	_, _, line, _ := runtime.Caller(skip)
	begin, end := t.curTag, line-1
	if len(t.tags) > 0 {
		begin += 1 //耗时不包括当前Tag行,但是需要记录从New到Tag的耗时
	}
	if end < begin {
		end = begin
	}
	info := tagInfo{
		tagLine:   line,
		beginLine: begin,
		endLine:   end,
		cost:      now - t.stepT,
	}
	if len(msg) > 0 {
		if str, ok := msg[0].(string); !ok {
			panic("current just support string type")
		} else {
			info.msg = str
		}
	}
	t.tags = append(t.tags, info)
	//更新当前
	t.curTag = line
	t.stepT = mclock.Now() //已此时间为准
	t.traceCost += t.stepT - now
}
func (t *TraceTime) Reset(args ...interface{}) *TraceTime {
	if !Enable || !t.enable {
		return t
	}
	t.Stop()

	//start
	_, _, line, _ := runtime.Caller(1)

	t.start = time.Now()
	t.beginT = mclock.Now()
	t.stepT = t.beginT
	t.curTag = line
	t.tags = nil
	t.args = nil
	t.AddArgs(args...)
	t.traceCost = mclock.Now() - t.beginT

	return t
}

func (t *TraceTime) Stop() {
	if !Enable || !t.enable {
		return
	}
	t.end = time.Now()

	if t.printToFile {
		t.print2()
		return
	}

	if len(t.tags) == 0 { //说明中间无Tag，只记录一次耗时
		now := mclock.Now()
		if t.minCost > 0 && now-t.beginT < t.minCost { //不需要打印
			return
		}
		t.logger.Info("trace call time:"+t.functionID,
			append(t.args,
				"sum.cost", common.PrettyDuration(now-t.beginT),
				"trace.cost", common.PrettyDuration(t.traceCost))...)
		return
	}

	//存在多个Tag时处理
	t.tag(2) //最后一个Tag
	now := mclock.Now()
	if t.minCost > 0 && now-t.beginT < t.minCost { //不需要打印
		return
	}
	content := bytes.NewBuffer(nil)
	for _, v := range t.tags {
		content.WriteString(fmt.Sprintf("%s[%d-%d]:%s,", v.msg, v.beginLine, v.endLine, common.PrettyDuration(v.cost)))
	}
	end := mclock.Now()
	t.traceCost += end - now
	t.logger.Info("trace call time:"+t.functionID,
		append(t.args,
			"line.cost"+strconv.Itoa(len(t.tags)), content.String(),
			"sum.cost", common.PrettyDuration(end-t.beginT),
			"trace.cost", common.PrettyDuration(t.traceCost))...)

}

func (t *TraceTime) print2() {
	now := mclock.Now()
	if t.minCost > 0 && now-t.beginT < t.minCost { //不需要打印
		return
	}

	if len(t.tags) > 0 { //存在多个Tag时处理
		t.tag(3) //最后一个Tag
	}

	args := []interface{}{
		"func", t.functionID,
	}

	for _, v := range t.tags {
		if v.msg == "" {
			args = append(args, fmt.Sprintf("tag_%d", v.tagLine), v.cost)
		} else {
			args = append(args, fmt.Sprintf("tag_%d_%s", v.tagLine, v.msg), v.cost)
		}
	}

	//不能修改顺序，为了计算日志调用耗时，采取延迟计算方式统计
	var endTime time.Time
	args = append(args,
		"start_t", t.start,
		"end_t", delayF(func() interface{} {
			endTime = time.Now() //更新截止时间
			return endTime
		}),
		"trace_cost", delayF(func() interface{} {
			return t.traceCost + mclock.AbsTime(endTime.Sub(t.end))
		}),
		"sum_cost", delayF(func() interface{} {
			return mclock.Now() - t.beginT
		}),
	)
	t.AddArgs(args...)
	t.logger.Info("tt", t.args...)
}

func Info(args ...interface{}) {
	if !Enable {
		return
	}
	log.Info("tt", append(args, "start_t", time.Now().UTC().UnixNano())...)
}

type myValue struct {
	v interface{}
}

type Stringer interface {
	String() string
}

type delayF func() interface{}

func (v myValue) String() string {
	switch v := v.v.(type) {
	case time.Duration:
		return strconv.FormatFloat(float64(v)/float64(time.Millisecond), 'f', -1, 64)
	case mclock.AbsTime:
		return strconv.FormatFloat(float64(v)/float64(time.Millisecond), 'f', -1, 64)
	case time.Time:
		return strconv.FormatInt(v.UTC().UnixNano(), 10)
	case string:
		return v
	case delayF:
		return myValue{v()}.String()
	case Stringer:
		return v.String()
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
