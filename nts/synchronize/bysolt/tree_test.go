package bysolt

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

func TestCalcPointNumber(t *testing.T) {
	start, err := time.Parse(time.RFC3339Nano, "2019-05-30T17:32:45+08:00")
	require.NoError(t, err)

	for number := uint64(1); number < 100; number += 3 {
		begin, end := PointTimeRange(start, number)
		//此时间内的所有计算都应该正确

		//开始之前，属于上一个区间
		caces := []struct {
			Time   time.Time
			Number uint64
		}{
			{begin.Add(-1), number - 1},
			{begin, number},
			{begin.Add(1), number},

			{end.Add(-1), number},
			{end, number + 1},
			{end.Add(1), number + 1},
		}

		for index, c := range caces {
			n := CalcPointNumber(start, c.Time)
			require.Equal(t, c.Number, n, "at %d,number %d, on time %s。[%s,%s]",
				index, number, c.Time, begin, end)
		}
	}

}

func TestPointTimeRange(t *testing.T) {

	parse := func(ts string) time.Time {
		tv, err := time.Parse(time.RFC3339Nano, ts)
		require.NoError(t, err)
		return tv
	}

	caces := []struct {
		start      string
		number     uint64
		begin, end string
	}{
		{
			start:  "2019-05-30T17:32:45.3235276+08:00",
			number: 1,
			begin:  "2019-05-30T17:32:45+08:00",
			end:    "2019-05-30T18:32:45+08:00",
		},
		{
			start:  "2019-05-30T17:32:45+08:00",
			number: 1,
			begin:  "2019-05-30T17:32:45+08:00",
			end:    "2019-05-30T18:32:45+08:00",
		},
		{
			start:  "2019-05-30T17:32:45+08:00",
			number: 2,
			begin:  "2019-05-30T18:32:45+08:00",
			end:    "2019-05-30T19:32:45+08:00",
		},
		{
			start:  "2019-05-30T23:32:45+08:00",
			number: 2,
			begin:  "2019-05-31T00:32:45+08:00",
			end:    "2019-05-31T01:32:45+08:00",
		},
	}

	for _, c := range caces {

		start := parse(c.start)

		begin, end := PointTimeRange(start, c.number)
		require.Equal(t, parse(c.begin), begin)
		require.Equal(t, parse(c.end), end)
	}
}

func toTime(ts string) time.Time {
	tv, err := time.ParseInLocation("2006-01-02 15:04:05", "2019-01-01 "+ts, time.UTC)
	if err != nil {
		panic(err)
	}
	return tv
}
func toUint64(ts string) uint64 {
	tv := toTime(ts)
	return uint64(tv.UnixNano())
}

func getNext(items []UnitInfo) <-chan UnitInfo {
	c := make(chan UnitInfo)
	go func() {
		for _, v := range items {
			c <- v
		}
		close(c)
	}()
	return c
}
func newUnit(time string) UnitInfo {
	return UnitInfo{
		Time: toUint64(time),
		Hash: common.StringToHash(time),
	}
}

func timeRange(begin, end string) TimeSize {
	return TimeSize{
		Begin: toTime(begin),
		End:   toTime(end),
	}
}

func TestFindNodes(t *testing.T) {
	//准备数据，最多 5 个数据一组，按 10 分钟切割

	ck := func(t *testing.T, timeSize TimeSize, items []UnitInfo, node []SlotNode) {
		var allInfo []UnitInfo
		for _, v := range node {
			allInfo = append(allInfo, v.Chains...)
		}
		require.Equal(t, len(items), len(allInfo))
		require.Equal(t, items, allInfo)

		tree, err := CreateTree(timeSize, node)
		require.NoError(t, err)
		//检查数据
		leafs := checkTree(t, timeSize, tree, 0)
		for i := 0; i < len(items) && i < len(leafs); i++ {
			require.Equal(t, items[i], leafs[i], "at index %d", i)
		}
		if len(items) != len(leafs) {
			lastIndex := len(items) - 1
			if len(leafs)-1 < lastIndex {
				lastIndex = len(leafs) - 1
			}
			diff := bytes.NewBuffer(nil)
			fmt.Fprintf(diff, "last item %d\n", lastIndex)
			diff.WriteString("items:")
			for i := lastIndex; i < len(items); i++ {
				diff.WriteString(time.Unix(0, int64(items[i].Time)).Format("15:04:05.000000"))
				diff.WriteString(",")
			}
			diff.WriteString("\n")
			diff.WriteString("leafs:")
			for i := lastIndex; i < len(leafs); i++ {
				diff.WriteString(time.Unix(0, int64(leafs[i].Time)).Format("15:04:05.000000"))
				diff.WriteString(",")
			}
			diff.WriteString("\n")
			t.Log(diff.String())
		}
		require.Equal(t, len(items), len(leafs))
		require.Equal(t, items, leafs)
	}

	t.Run("empty", func(t *testing.T) {
		timeSize := timeRange("01:00:00", "02:00:00")
		all := FindNodes(timeSize, 10, nil, getNext(nil))
		//
		require.Len(t, all, 0)
	})
	t.Run("one item", func(t *testing.T) {
		//测试，整个区间只有一个数据
		items := []UnitInfo{
			newUnit("01:32:45"),
		}
		timeSize := timeRange("01:00:00", "02:00:00")

		all := FindNodes(timeSize, 10, nil, getNext(items))
		//
		require.Len(t, all, 1)
		require.Equal(t, all[0], SlotNode{
			Chains:    items,
			TimeRange: timeSize,
		})
		ck(t, timeSize, items, all)
	})
	t.Run("towLevel", func(t *testing.T) {
		//测试，由左右两组数据组成的
		timeSize := timeRange("01:00:00", "02:00:00")
		//整个区间是 1 小时，因此需要按 30 分钟分成两部分
		items := []UnitInfo{
			newUnit("01:00:01"),
			newUnit("01:12:45"),
			newUnit("01:30:00"),
			newUnit("01:32:45"),
			newUnit("01:32:46"),
		}

		all := FindNodes(timeSize, 3, nil, getNext(items))

		wants := []SlotNode{
			{
				TimeRange: timeSize.Left(),
				Chains: []UnitInfo{
					newUnit("01:00:01"),
					newUnit("01:12:45"),
				},
			},
			{
				TimeRange: timeSize.Right(),
				Chains: []UnitInfo{
					newUnit("01:30:00"),
					newUnit("01:32:45"),
					newUnit("01:32:46"),
				},
			},
		}
		wantStr := formatOutputNodes(wants).String()
		gotStr := formatOutputNodes(all).String()
		if wantStr != gotStr {
			t.Fatalf("\nexpected:\n%s\nactual  :%s", wantStr, gotStr)
		}
		ck(t, timeSize, items, all)
	})
	t.Run("more", func(t *testing.T) {
		//测试，由左右两组数据组成的
		timeSize := TimeSize{
			Begin: toTime("01:00:00"),
			End:   toTime("02:00:00"),
		}
		//整个区间是 1 小时，因此需要按 30 分钟分成两部分
		items := []UnitInfo{
			newUnit("01:00:00"),
			newUnit("01:00:01"),
			newUnit("01:12:45"),
			newUnit("01:29:59"),
			newUnit("01:32:45"),
			newUnit("01:32:46"),
		}

		all := FindNodes(timeSize, 3, nil, getNext(items))

		wants := []SlotNode{
			{
				TimeRange: timeRange("01:00:00", "01:15:00"),
				Chains: []UnitInfo{
					newUnit("01:00:00"),
					newUnit("01:00:01"),
					newUnit("01:12:45"),
				},
			},
			{
				TimeRange: timeRange("01:15:00", "01:30:00"),
				Chains: []UnitInfo{
					newUnit("01:29:59"),
				},
			},
			{
				TimeRange: timeRange("01:30:00", "02:00:00"),
				Chains: []UnitInfo{
					newUnit("01:32:45"),
					newUnit("01:32:46"),
				},
			},
		}
		wantStr := formatOutputNodes(wants).String()
		gotStr := formatOutputNodes(all).String()
		if wantStr != gotStr {
			t.Fatalf("\nexpected:\n%s\nactual  :%s", wantStr, gotStr)
		}
		ck(t, timeSize, items, all)
	})
	t.Run("more2", func(t *testing.T) {
		//测试，由左右两组数据组成的
		timeSize := TimeSize{
			Begin: toTime("01:00:00"),
			End:   toTime("02:00:00"),
		}
		//整个区间是 1 小时，因此需要按 30 分钟分成两部分
		items := []UnitInfo{
			newUnit("01:00:00"),
			newUnit("01:00:01"),
			newUnit("01:12:45"),
			newUnit("01:29:59"),
			newUnit("01:30:00"),
			newUnit("01:32:46"),
			newUnit("01:32:47"),
			newUnit("01:45:47"),
		}

		all := FindNodes(timeSize, 3, nil, getNext(items))

		wants := []SlotNode{
			{
				TimeRange: timeRange("01:00:00", "01:15:00"),
				Chains: []UnitInfo{
					newUnit("01:00:00"),
					newUnit("01:00:01"),
					newUnit("01:12:45"),
				},
			},
			{
				TimeRange: timeRange("01:15:00", "01:30:00"),
				Chains: []UnitInfo{
					newUnit("01:29:59"),
				},
			},
			{
				TimeRange: timeRange("01:30:00", "01:45:00"),
				Chains: []UnitInfo{
					newUnit("01:30:00"),
					newUnit("01:32:46"),
					newUnit("01:32:47"),
				},
			},
			{
				TimeRange: timeRange("01:45:00", "02:00:00"),
				Chains: []UnitInfo{
					newUnit("01:45:47"),
				},
			},
		}
		wantStr := formatOutputNodes(wants).String()
		gotStr := formatOutputNodes(all).String()
		if wantStr != gotStr {
			t.Fatalf("\nexpected:\n%s\nactual  :%s", wantStr, gotStr)
		}
		ck(t, timeSize, items, all)
	})

	t.Run("more3", func(t *testing.T) {
		//测试，由左右两组数据组成的
		timeSize := timeRange("01:00:00", "02:00:00")
		//整个区间是 1 小时，因此需要按 30 分钟分成两部分
		items := []UnitInfo{
			newUnit("01:00:00"),
			newUnit("01:00:01"),
			newUnit("01:12:45"),
			newUnit("01:29:59"),
			newUnit("01:30:00"),
			newUnit("01:32:46"),
			newUnit("01:32:47"),
			newUnit("01:37:47"),
			newUnit("01:45:00"),
			newUnit("01:45:47"),
		}

		all := FindNodes(timeSize, 3, nil, getNext(items))
		wants := []SlotNode{
			{
				TimeRange: timeRange("01:00:00", "01:15:00"),
				Chains: []UnitInfo{
					newUnit("01:00:00"),
					newUnit("01:00:01"),
					newUnit("01:12:45"),
				},
			},
			{
				TimeRange: timeRange("01:15:00", "01:30:00"),
				Chains: []UnitInfo{
					newUnit("01:29:59"),
				},
			},
			{
				TimeRange: timeRange("01:30:00", "01:37:30"),
				Chains: []UnitInfo{
					newUnit("01:30:00"),
					newUnit("01:32:46"),
					newUnit("01:32:47"),
				},
			},
			{
				TimeRange: timeRange("01:37:30", "01:45:00"),
				Chains: []UnitInfo{
					newUnit("01:37:47"),
				},
			},
			{
				TimeRange: timeRange("01:45:00", "02:00:00"),
				Chains: []UnitInfo{
					newUnit("01:45:00"),
					newUnit("01:45:47"),
				},
			},
		}
		wantStr := formatOutputNodes(wants).String()
		gotStr := formatOutputNodes(all).String()
		if wantStr != gotStr {
			t.Fatalf("\nexpected:\n%s\nactual  :%s", wantStr, gotStr)
		}

		ck(t, timeSize, items, all)
	})
	t.Run("sameTime", func(t *testing.T) {
		//测试，由左右两组数据组成的
		timeSize := timeRange("01:00:00", "02:00:00")
		//整个区间是 1 小时，因此需要按 30 分钟分成两部分
		items := []UnitInfo{
			newUnit("01:00:00"),
			newUnit("01:00:01"),
			newUnit("01:00:01"),
			newUnit("01:12:45"),
			newUnit("01:29:59"),
			newUnit("01:30:00"),
			newUnit("01:32:46"),
			newUnit("01:32:47"),
			newUnit("01:37:47"),
			newUnit("01:45:00"),
			newUnit("01:45:00"),
			newUnit("01:45:47"),
		}

		all := FindNodes(timeSize, 3, nil, getNext(items))
		wants := []SlotNode{
			{
				TimeRange: timeRange("01:00:00", "01:07:30"),
				Chains: []UnitInfo{
					newUnit("01:00:00"),
					newUnit("01:00:01"),
					newUnit("01:00:01"),
				},
			},
			{
				TimeRange: timeRange("01:07:30", "01:15:00"),
				Chains: []UnitInfo{
					newUnit("01:12:45"),
				},
			},
			{
				TimeRange: timeRange("01:15:00", "01:30:00"),
				Chains: []UnitInfo{
					newUnit("01:29:59"),
				},
			},
			{
				TimeRange: timeRange("01:30:00", "01:37:30"),
				Chains: []UnitInfo{
					newUnit("01:30:00"),
					newUnit("01:32:46"),
					newUnit("01:32:47"),
				},
			},
			{
				TimeRange: timeRange("01:37:30", "01:45:00"),
				Chains: []UnitInfo{
					newUnit("01:37:47"),
				},
			},
			{
				TimeRange: timeRange("01:45:00", "02:00:00"),
				Chains: []UnitInfo{
					newUnit("01:45:00"),
					newUnit("01:45:00"),
					newUnit("01:45:47"),
				},
			},
		}

		wantStr := formatOutputNodes(wants).String()
		gotStr := formatOutputNodes(all).String()
		if wantStr != gotStr {
			t.Fatalf("\nexpected:\n%s\nactual  :%s", wantStr, gotStr)
		}

		ck(t, timeSize, items, all)
	})
	t.Run("tomany", func(t *testing.T) {
		//0 1h0m0s 1
		//1 30m0s 2
		//2 15m0s 4
		//3 7m30s 8
		//4 3m45s 16
		//5 1m52.5s 32
		//6 56.25s 64
		//7 28.125s 128
		//8 14.0625s 256
		//9 7.03125s 512
		//10 3.515625s 1024
		//11 1.7578125s 2048
		//12 878.90625ms 4096
		timeSize := timeRange("01:00:05", "02:00:05")
		//每秒一笔交易
		var items []UnitInfo
		for begin := timeSize.Begin; begin.Before(timeSize.End); begin = begin.Add(time.Second) {
			items = append(items, newUnit(begin.Format("15:04:05")))
		}
		for maxSize := 1; maxSize <= len(items); maxSize++ {
			//最大允许 15 笔交易，则
			all := FindNodes(timeSize, maxSize, nil, getNext(items))
			ck(t, timeSize, items, all)
		}
	})
}

func TestCreateTree(t *testing.T) {
	timeSize := timeRange("01:00:00", "02:00:00")
	nodes := []SlotNode{
		{

			TimeRange: timeRange("01:00:00", "01:15:00"),
			Chains: []UnitInfo{
				newUnit("01:00:00"),
				newUnit("01:00:01"),
				newUnit("01:12:45"),
			},
		},
		{

			TimeRange: timeRange("01:15:00", "01:30:00"),
			Chains: []UnitInfo{
				newUnit("01:30:00"),
			},
		},
		{

			TimeRange: timeRange("01:30:00", "01:37:30"),
			Chains: []UnitInfo{
				newUnit("01:32:45"),
				newUnit("01:32:46"),
				newUnit("01:32:47"),
			},
		},
		{

			TimeRange: timeRange("01:37:30", "01:45:00"),
			Chains: []UnitInfo{
				newUnit("01:37:47"),
				newUnit("01:45:00"),
			},
		},
		{

			TimeRange: timeRange("01:45:00", "02:00:00"),
			Chains: []UnitInfo{
				newUnit("01:45:47"),
			},
		},
	}

	var chains []UnitInfo
	for _, v := range nodes {
		chains = append(chains, v.Chains...)
	}

	tree, err := CreateTree(timeSize, nodes)
	require.NoError(t, err)
	//检查数据
	leafs := checkTree(t, timeSize, tree, 0)
	require.Equal(t, chains, leafs)

	buf := bytes.NewBuffer(nil)
	printTree(tree, buf)
	t.Log(buf.String())
}

func TestEmptyTree(t *testing.T) {
	timeSize := timeRange("01:00:00", "02:00:00")
	nodes := []SlotNode{}

	tree, err := CreateTree(timeSize, nodes)
	require.NoError(t, err)
	//检查数据
	leafs := checkTree(t, timeSize, tree, 0)
	require.Zero(t, leafs)

}

func TestOntNodeTree(t *testing.T) {
	timeSize := timeRange("01:00:00", "01:10:00")

	//整个区间是 1 小时，因此需要按 30 分钟分成两部分
	items := []UnitInfo{
		newUnit("01:00:10"),
		newUnit("01:01:20"),
		newUnit("01:01:30"),
		newUnit("01:02:40"),
		newUnit("01:02:50"),
		newUnit("01:09:51"),
	}

	all := FindNodes(timeSize, maxSlotUnits, nil, getNext(items))
	tree, err := CreateTree(timeSize, all)
	require.NoError(t, err)
	require.Equal(t, items, tree.Value)
}

func TestCalcDepth(t *testing.T) {
	timeSize := timeRange("01:00:00", "02:00:00")

	for depth := 0; depth < 20; depth++ {
		items := 1 << uint(depth)
		unit := timeSize.Size() / time.Duration(items)

		fmt.Print(depth, unit, items, unit)

		gotDepth := CalcDepth(timeSize.Size(), unit)
		require.Equal(t, depth, int(gotDepth))

		//start := timeSize.Begin
		//for i := 0; i < items; i++ {
		//	//fmt.Print(TimeRange{start, start.Add(unit)})
		//	//fmt.Print(",")
		//	start = start.Add(unit)
		//}
		fmt.Println("")
	}

}

func TestCreateTree2(t *testing.T) {
	timeSize := timeRange("01:00:00", "02:00:00")

	unit := timeSize.Size() / (1 << 4)
	start := timeSize.Begin
	for i := 0; i < 1<<4; i++ {
		t.Log(i, TimeSize{start, start.Add(unit)})
		start = start.Add(unit)
	}

	nodes := []SlotNode{
		{

			TimeRange: timeRange("01:07:30", "01:15:00"),
			Chains: []UnitInfo{
				newUnit("01:12:45"),
			},
		},
		{

			TimeRange: timeRange("01:22:30", "01:26:15"),
			Chains: []UnitInfo{
				newUnit("01:23:00"),
			},
		},
		{

			TimeRange: timeRange("01:26:15", "01:30:00"),
			Chains: []UnitInfo{
				newUnit("01:23:00"),
			},
		},
		{

			TimeRange: timeRange("01:37:30", "01:41:15"),
			Chains: []UnitInfo{
				newUnit("01:38:46"),
				newUnit("01:40:40"),
			},
		},
		{

			TimeRange: timeRange("01:41:15", "01:45:00"),
			Chains: []UnitInfo{
				newUnit("01:45:00"),
			},
		},
		{

			TimeRange: timeRange("01:45:00", "02:00:00"),
			Chains: []UnitInfo{
				newUnit("01:45:47"),
			},
		},
	}
	//4[01:00:00,01:03:45],[01:03:45,01:07:30],[01:07:30,01:11:15],[01:11:15,01:15:00],[01:15:00,01:18:45],[01:18:45,01:22:30],[01:22:30,01:26:15],[01:26:15,01:30:00],[01:30:00,01:33:45],[01:33:45,01:37:30],[01:37:30,01:41:15],[01:41:15,01:45:00],[01:45:00,01:48:45],[01:48:45,01:52:30],[01:52:30,01:56:15],[01:56:15,02:00:00],

	var chains []UnitInfo
	for _, v := range nodes {
		chains = append(chains, v.Chains...)
	}

	tree, err := CreateTree(timeSize, nodes)
	require.NoError(t, err)

	//检查数据
	leafs := checkTree(t, timeSize, tree, 0)
	require.Equal(t, chains, leafs)
}

func checkTree(t testing.TB, timeSize TimeSize, tree *SlotTree, depth uint64) []UnitInfo {
	require.Equal(t, timeSize.String(), tree.TimeRange.String())

	var leafs []UnitInfo
	if tree.Left != nil {
		leafs = append(leafs,
			checkTree(t, timeSize.Left(), tree.Left, depth+1)...,
		)
	}
	if tree.Right != nil {
		leafs = append(leafs,
			checkTree(t, timeSize.Right(), tree.Right, depth+1)...,
		)
	}
	switch v := tree.Value.(type) {
	case []UnitInfo:
		leafs = append(leafs, v...)
	}
	return leafs
}

func printTree(tree *SlotTree, buf *bytes.Buffer) {

	//top
	hash := tree.Hash()
	buf.Write(hash[:])
	fmt.Fprintf(buf, "[%s,%s]",
		tree.TimeRange.Begin.Format("15:04:05"),
		tree.TimeRange.End.Format("15:04:05"))
	buf.WriteString("\n\t")
	if tree.Left == nil {
		buf.WriteString("left=nil\n")
	} else {
		printTree(tree.Left, buf)
	}
	if tree.Right == nil {
		buf.WriteString("right=nil\n")
	} else {
		printTree(tree.Right, buf)
	}
}

type formatOutputNodes []SlotNode

func (list formatOutputNodes) String() string {
	b := bytes.NewBuffer(nil)
	b.WriteString("[")
	for _, v := range list {
		b.WriteString("\n\t")
		fmt.Fprintf(b, `depth=%d,range=[%s,%s]%s,chains=%d [`, v.Depth(time.Hour),
			v.TimeRange.Begin.Format("15:04:05.999999"),
			v.TimeRange.End.Format("15:04:05.999999"),
			v.TimeRange.Size(),
			len(v.Chains))
		for _, c := range v.Chains {
			b.WriteString(time.Unix(0, int64(c.Time)).Format("15:04:05.999999"))
			b.WriteString("\t,")
		}
	}
	b.WriteString("\n]")
	return b.String()
}

func BenchmarkFindNodes(b *testing.B) {
	//不同数据下创建树的数读
	timeSize := timeRange("01:00:05", "02:00:05")

	unitC := make(chan UnitInfo)

	go func() {
		utime := timeSize.Begin
		for i := 0; i < b.N; i++ {
			unitC <- UnitInfo{
				Time: uint64(utime.UnixNano()),
			}
			utime = utime.Add(time.Second / 1000)
			if utime.After(timeSize.End) {
				utime = timeSize.End
			}
		}
		close(unitC)
	}()
	b.ReportAllocs()
	b.ResetTimer()
	FindNodes(timeSize, maxSlotUnits, nil, unitC)
}
