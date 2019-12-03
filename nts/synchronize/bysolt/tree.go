package bysolt

import (
	"fmt"
	"math"
	"sort"
	"time"

	"gitee.com/nerthus/nerthus/log"
)

const (
	pointTimeSpan = time.Hour //一个时间点的时间跨度
	maxSlotUnits  = 10000     //一个插槽分片中最大存放的单元数量
)

func PointTimeRange(initTime time.Time, number uint64) (begin, end time.Time) {
	if number == 0 {
		panic("number is zero")
	}

	initTime = time.Unix(initTime.Unix(), 0) //必须使用整点秒
	//一小时一个区间
	times := time.Duration(number-1) * pointTimeSpan
	return initTime.Add(times), initTime.Add(times + pointTimeSpan)
}
func CalcDepth(sum time.Duration, cur time.Duration) uint64 {
	//根据计算方式反推 深度
	return uint64(math.Ceil(math.Log10(float64(sum/cur)) / math.Log10(2)))
}

// 计算时间属于第几个区间
func CalcPointNumber(initTime time.Time, utime time.Time) uint64 {
	initTime = time.Unix(initTime.Unix(), 0) //必须使用整点秒
	diff := utime.Sub(initTime)
	if diff < 0 {
		return 0
	}
	if diff < pointTimeSpan {
		return 1
	} else if diff == pointTimeSpan {
		return 2
	}
	//取证加 1
	return uint64(math.Floor(float64(diff)/float64(pointTimeSpan))) + 1
}

// 时间区间
type TimeSize struct {
	Begin, End time.Time
}

func (ts TimeSize) Middle() time.Time {
	return ts.Begin.Add(ts.End.Sub(ts.Begin) / 2)
}
func (ts TimeSize) Left() TimeSize {
	return TimeSize{
		Begin: ts.Begin,
		End:   ts.Middle(),
	}
}
func (ts TimeSize) Right() TimeSize {
	return TimeSize{
		Begin: ts.Middle(),
		End:   ts.End,
	}
}
func (ts TimeSize) Next() TimeSize {
	return TimeSize{
		Begin: ts.End,
		End:   ts.End.Add(ts.Size()),
	}
}
func (ts TimeSize) Size() time.Duration {
	return ts.End.Sub(ts.Begin)
}
func (ts TimeSize) String() string {
	return fmt.Sprintf("[%s,%s]",
		ts.Begin.Format("15:04:05"),
		ts.End.Format("15:04:05"))
}

func (n SlotNode) Depth(t time.Duration) uint64 {
	return CalcDepth(t, n.TimeRange.Size())
}

func FindNodes(timeRange TimeSize, maxSize int, loads []UnitInfo, next <-chan UnitInfo) (nodes []SlotNode) {
	middle, endTime := uint64(timeRange.Middle().UnixNano()), uint64(timeRange.End.UnixNano())

	//防止对接在非常小的位置上
	if timeRange.Size() <= time.Microsecond {
		//如果间隔已非常小，则直接作为一个整体返回
		for info := range next {
			loads = append(loads, info)
		}
		return []SlotNode{
			{
				Chains:    loads,
				TimeRange: timeRange,
			},
		}
	}

	var (
		lefts  = make([]UnitInfo, 0, maxSize)
		rights = make([]UnitInfo, 0, maxSize)
		others []UnitInfo
	)
	//分割
	for _, info := range loads {
		if info.Time < middle {
			lefts = append(lefts, info)
		} else if info.Time < endTime {
			rights = append(rights, info)
		} else {
			others = append(others, info)
		}
	}
	//继续追加，直到超过数据上限，和已确定是否有更多other
	for next != nil &&
		((len(rights)+len(lefts)+len(others)) < maxSize+1 || len(others) == 0) {
		n, ok := <-next
		if !ok || n.Time == 0 {
			break
		}
		if n.Time < middle {
			lefts = append(lefts, n)
		} else if n.Time < endTime {
			rights = append(rights, n)
		} else {
			others = append(others, n)
		}
	}
	//fmt.Println("2", len(rights)+len(lefts)+len(others))

	log.Trace("findNodes", "range", timeRange,
		"left.len", len(lefts), "right.len", len(lefts), "other.len", len(others))

	if len(rights) == 0 && len(lefts) == 0 && len(others) == 0 {
		return nil
	}
	//此节点已无新数据
	if len(others) == 0 && (len(rights)+len(lefts)) <= maxSize {
		if len(rights)+len(lefts) == 0 {
			return nil
		}
		return []SlotNode{
			{
				Chains:    append(lefts, rights...),
				TimeRange: timeRange,
			},
		}
	}

	//如果左边已超过上限，则必须进行拆分
	if len(rights)+len(others) > 0 { //左边已无其他数据，可以独立拆分
		rightNodes := FindNodes(timeRange.Left(), maxSize, lefts, nil)
		nodes = append(nodes, rightNodes...)
	} else { //也许左边还有更多数据，允许继续提取
		rightNodes := FindNodes(timeRange.Left(), maxSize, lefts, next)
		nodes = append(nodes, rightNodes...)
	}

	//检查右边
	if len(others) > 0 { //右边已无其他数据，可以独立拆分
		nodes = append(nodes, FindNodes(timeRange.Right(), maxSize, rights, nil)...)
	} else { //也许右边还有更多数据，允许继续提取
		nodes = append(nodes, FindNodes(timeRange.Right(), maxSize, rights, next)...)
	}

	//other 必然属于 当前时间范围的下一个时间范围内。
	if len(others) > 0 {
		nodes = append(nodes, FindNodes(timeRange.Next(), maxSize, others, next)...)
	}

	return nodes
}

type SlotTree struct {
	Value     interface{}
	Left      *SlotTree
	Right     *SlotTree
	TimeRange TimeSize

	hash Hash
}

var emptyTree = new(SlotTree)

func (tree *SlotTree) Children() (Node, Node) {
	l, r := tree.Left, tree.Right
	if l == nil {
		l = emptyTree
	}
	if r == nil {
		r = emptyTree
	}
	return l, r
}

func (tree *SlotTree) IsLeaf() bool {
	if tree.Right == nil && tree.Left == nil {
		return true
	}
	return false
}
func (tree *SlotTree) Timestamp() TimeSize {
	return tree.TimeRange
}

func (tree *SlotTree) Hash() Hash {
	if tree.Value == nil && tree.Right == nil && tree.Left == nil {
		return emptyHash
	}
	if tree.hash != emptyHash {
		return tree.hash
	}
	if tree.Value == nil {
		//通过左右节点计算
		if tree.Left == nil && tree.Right == nil {
			panic("should be have left and right tree")
		}
		tree.hash = CalcHash(tree.LeftHash().Bytes(), tree.RightHash().Bytes())
	} else {
		switch v := tree.Value.(type) {
		default:
			panic("game over")
		case Hash:
			tree.hash = v
		case []UnitInfo:
			var index int
			tree.hash = CalcHashMore(func() ([]byte, bool) {
				if index >= len(v) {
					return nil, false
				}
				b := v[index].Hash.Bytes()
				index++
				return b, true
			})
		}
	}
	return tree.hash
}
func (tree *SlotTree) LeftHash() Hash {
	if tree.Left == nil {
		return Hash{}
	}
	return tree.Left.Hash()
}
func (tree *SlotTree) RightHash() Hash {
	if tree.Right == nil {
		return Hash{}
	}
	return tree.Right.Hash()
}

func CreateTree(timeSize TimeSize, list []SlotNode) (*SlotTree, error) {

	//优先处理最深处的数据
	depths := make(map[uint64][]*SlotTree)
	var maxDepth uint64

	maxTimeRange := timeSize.Size()

	for _, v := range list {
		depth := v.Depth(maxTimeRange)
		depths[depth] = append(depths[depth], &SlotTree{
			Value:     v.Chains,
			TimeRange: v.TimeRange,
		})
		if maxDepth < depth {
			maxDepth = depth
		}
	}

	log.Trace("createTree", "range", timeSize, "maxDepth", maxDepth, "len", len(depths[maxDepth]))
	for {
		curr := depths[maxDepth]
		if maxDepth == 0 {
			if len(curr) == 0 {
				//一个节点也没有则构建一个空树
				return &SlotTree{
					TimeRange: timeSize,
				}, nil
			} else if len(curr) != 1 {
				//不正确
				panic(fmt.Errorf("the tree root should be have one node,but got %d", len(curr)))
			}
			return curr[0], nil
		}

		for index := 0; index < len(curr); {
			a := curr[index]
			var b *SlotTree
			if index+1 < len(curr) {
				b = curr[index+1]

				//同一层中，节点的时间跨度一致
				if a.TimeRange.Size() != b.TimeRange.Size() {
					return nil, fmt.Errorf("the time range id diffrent at depth %d: a(%s)=%s,b(%s)=%s",
						maxDepth,
						a.TimeRange, a.TimeRange.Size(),
						b.TimeRange, b.TimeRange.Size())
				}
			}
			//计算出所在位置
			leftLoc := int(a.TimeRange.Begin.Sub(timeSize.Begin)/(a.TimeRange.Size()) + 1)

			var rightLoc int
			if b != nil {
				rightLoc = int(b.TimeRange.Begin.Sub(timeSize.Begin)/(b.TimeRange.Size()) + 1)
			}
			var node SlotTree
			//判断两种是否属于同一组的左右节点
			if leftLoc+1 == rightLoc && leftLoc%2 == 1 {
				//当前第一个为左节点，且另一个相邻时，说明两则属于同一组
				// 类似： D 和 E 或者 F 和 G
				/*
							   A
							____|_____
							|        |
							B        C
					    ____|____  ____|____
					    |       |  |       |
						D       E  F       G
				*/
				//构成新节点
				node = SlotTree{
					Left:  a,
					Right: b,
					TimeRange: TimeSize{
						Begin: a.TimeRange.Begin,
						End:   b.TimeRange.End,
					},
				}
				index += 2 //跳过已处理的 D和 E
				log.Trace("new node", "range", node.TimeRange,
					"left", a.TimeRange, "right", b.TimeRange)
			} else {
				//两则不相邻
				/*  D 和 F 情况
						   A                   	            A                           A
						____|_____             			____|_____                 	____|_____
						|        |             			|        |                 	|        |
						B        C             			B        C                 	B        C
				    ____|____  ____|____       	    ____|____  ____|____        ____|____  ____|____
				    |       |  |       |       	    |       |  |       |       |       |  |       |
					D          E       F       		        D   F       G              D          F

				*/
				//不管属于什么情况
				//必然是将 D 独立 生成父节点
				if leftLoc%2 == 1 { //属于左节点
					node = SlotTree{
						Left:  a,
						Right: nil, //空
						TimeRange: TimeSize{
							Begin: a.TimeRange.Begin, //时间前移
							End:   a.TimeRange.End.Add(a.TimeRange.Size()),
						},
					}
					log.Trace("new node",
						"range", node.TimeRange, "left", a.TimeRange, "right", "nil")

				} else {
					node = SlotTree{
						Left:  nil,
						Right: a, //空
						TimeRange: TimeSize{
							Begin: a.TimeRange.Begin.Add(-a.TimeRange.Size()), //时间前移
							End:   a.TimeRange.End,
						},
					}
					log.Trace("new node",
						"range", node.TimeRange, "left", "nil", "right", a.TimeRange)
				}
				index += 1 //继续处理下一个 E
			}
			//提升层次
			preDepth := append(depths[maxDepth-1], &node)
			sort.Slice(preDepth, func(i, j int) bool {
				return preDepth[i].TimeRange.Begin.Before(preDepth[j].TimeRange.Begin)
			})
			depths[maxDepth-1] = preDepth
		}
		maxDepth--
	}
}
