package arbitration

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/graph"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/stretchr/testify/require"
)

type mockChainReader struct {
	tail        func(chain common.Address) *types.Header
	header      func(mcaddr common.Address, number uint64) *types.Header
	events      func(events ...interface{})
	unitNumber  func(hash common.Hash) (common.Address, uint64)
	wakChildren func(parentHash common.Hash, cb func(children common.Hash) bool)
}

func (m *mockChainReader) GetUnitState(unitHash common.Hash) (*state.StateDB, error) {
	panic("implement me")
}

func (m *mockChainReader) Config() *params.ChainConfig {
	return params.TestnetChainConfig
}

func (m *mockChainReader) GetChainTailHead(chain common.Address) *types.Header {
	if m.tail != nil {
		return m.tail(chain)
	}
	return nil
}

func (m *mockChainReader) GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header {
	if m.header != nil {
		return m.header(mcaddr, number)
	}
	return nil
}

func (m *mockChainReader) StateAt(chain common.Address, root common.Hash) (*state.StateDB, error) {
	panic("implement me")
}

func (m *mockChainReader) GetChainTailState(mcaddr common.Address) (*state.StateDB, error) {
	return nil, nil
}

func (m *mockChainReader) PostChainEvent(events ...interface{}) {
	if m.events != nil {
		m.events(events...)
	}
}

func (m *mockChainReader) GetUnitNumber(hash common.Hash) (common.Address, uint64) {
	if m.unitNumber != nil {
		return m.unitNumber(hash)
	}
	panic("implement me")
}

func (m *mockChainReader) WakChildren(parentHash common.Hash, cb func(children common.Hash) bool) {
	if m.wakChildren != nil {
		m.wakChildren(parentHash, cb)
		return
	}
	panic("implement me")
}

func TestOnFoundBad(t *testing.T) {
	db := ntsdb.NewMemDB()
	dc := mockChainReader{}

	//
	//                      B1<--------------B2<--------B3
	//						 ↓                ↓
	//    （A1)  <--------- (A2) <----------- A3<---------A4
	//		↑								   ↑
	//      C2<-------C3<-----------C4<------ C5<---------C6
	//                              ↓                     ↑
	//      D2<-------D3<-----------D4<------ D5<---------D6<-------D7
	//                                                    ↓
	//      E2<-------E3<-----------E4<------ E5<---------E6<-------E7
	//

	chains := []common.Address{
		common.StringToAddress("A"),
		common.StringToAddress("B"),
		common.StringToAddress("C"),
		common.StringToAddress("D"),
	}
	// 如上图，设定 A2 单元需要仲裁时，
	// 最终期望停摆结果是：A2,B1,C5,D6
	badChainNumber := ChainNumber{
		Chain:  common.StringToAddress("A"),
		Number: 2,
	}
	needStopped := []ChainNumber{
		{common.StringToAddress("A"), 2},
		{common.StringToAddress("B"), 1},
		{common.StringToAddress("C"), 5},
		{common.StringToAddress("D"), 6},
	}

	dagInfo := make(graph.Dag)
	dagInfo.AddEdge("A2", "A1")
	dagInfo.AddEdge("A3", "A2")
	dagInfo.AddEdge("A4", "A3")
	dagInfo.AddEdge("B2", "B1")
	dagInfo.AddEdge("B3", "B2")
	dagInfo.AddEdge("C3", "C2")
	dagInfo.AddEdge("C4", "C3")
	dagInfo.AddEdge("C5", "C4")
	dagInfo.AddEdge("C6", "C5")
	dagInfo.AddEdge("D7", "D6")
	dagInfo.AddEdge("D6", "D5")
	dagInfo.AddEdge("D5", "D4")
	dagInfo.AddEdge("D4", "D3")
	dagInfo.AddEdge("D3", "D2")
	dagInfo.AddEdge("E7", "E6")
	dagInfo.AddEdge("E6", "E5")
	dagInfo.AddEdge("E5", "E4")
	dagInfo.AddEdge("E4", "E3")
	dagInfo.AddEdge("E3", "E2")

	dagInfo.AddEdge("B1", "A2")
	dagInfo.AddEdge("B2", "A3")
	dagInfo.AddEdge("C5", "A3")
	dagInfo.AddEdge("C4", "D4")
	dagInfo.AddEdge("C2", "A1")
	dagInfo.AddEdge("D6", "C6")
	dagInfo.AddEdge("D6", "E6")

	for k, _ := range dagInfo.SubGraph("B3") {
		fmt.Print(k, ",")
	}
	// 链尾部数据
	chainTails := map[common.Address]uint64{
		common.StringToAddress("A"): 4,
		common.StringToAddress("B"): 3,
		common.StringToAddress("C"): 6,
		common.StringToAddress("D"): 7,
		common.StringToAddress("E"): 7,
	}

	units := make(map[common.Hash]types.Header)
	for k, _ := range dagInfo {
		number, err := strconv.ParseInt(string(k.(string)[1]), 10, 64)
		require.NoError(t, err)

		h := types.Header{
			MC:     common.StringToAddress(string(k.(string)[0])),
			Number: uint64(number),
		}
		units[h.Hash()] = h
		dagInfo[k].Val = h.Hash()
	}

	badUnits := []types.AffectedUnit{
		{
			Header: types.Header{MC: common.StringToAddress("A"), Number: 2},
			Votes: []types.WitenssVote{
				{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set(make([]byte, 50))},
			},
		},
		{
			Header: types.Header{MC: common.StringToAddress("A"), Number: 2, Timestamp: 1},
			Votes: []types.WitenssVote{
				{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set(make([]byte, 25))},
			},
		},
	}

	//准备和

	dc.events = func(events ...interface{}) {
		//for _, e := range events {
		//	ev := e.(types.ChainMCProcessStatusChangedEvent)
		//	//require.True(t, ok)
		//	t.Log("StopChain:", ev.MC, ev.Status)
		//}
	}
	dc.wakChildren = func(parentHash common.Hash, cb func(children common.Hash) bool) {
		h := units[parentHash]
		c := string(h.MC.Bytes()[len(h.MC.Bytes())-1:])
		id := fmt.Sprintf("%s%d", c, h.Number)

		info := bytes.NewBufferString("")

		fmt.Fprint(info, "Sub(", id, ")=")
		defer func() {
			fmt.Fprint(info, " >>\n")
			fmt.Print(info.String())
		}()
		var stop bool
		for _, v := range dagInfo.SubGraph(id) {
			if v.Parent() == nil {
				continue
			}
			fmt.Fprint(info, v.Name(), ",")
			if stop {
				continue
			}
			if v.Parent().Name() == dagInfo[id].Name() {
				fmt.Fprint(info, "(", v.Name(), "),")
				if cb(v.Val.(common.Hash)) {
					stop = true
				}
			}
		}
	}
	dc.header = func(mcaddr common.Address, number uint64) *types.Header {
		c := string(mcaddr.Bytes()[len(mcaddr.Bytes())-1:])
		id := fmt.Sprintf("%s%d", c, number)
		_, ok := dagInfo[id]
		require.True(t, ok, "should be found unit %d", id)
		return &types.Header{
			MC:     mcaddr,
			Number: number,
		}
	}
	dc.tail = func(chain common.Address) *types.Header {
		tail, ok := chainTails[chain]
		require.True(t, ok, "should be contains chain tail %s", chain.Hex())
		return &types.Header{
			MC:     chain,
			Number: tail,
		}
	}
	dc.unitNumber = func(hash common.Hash) (common.Address, uint64) {
		h := units[hash]
		return h.MC, h.Number

	}
	mockCheckProofWithState = func(signer types.Signer, rep []types.AffectedUnit) ([]common.Address, error) {
		return nil, nil
	}

	checkStopFlag := func(t *testing.T, want, got []ChainNumber) {
		//需要匹配
		//先排序
		sort.Slice(want, func(i, j int) bool {
			return want[i].Chain.String() < want[j].Chain.String()
		})
		sort.Slice(got, func(i, j int) bool {
			return got[i].Chain.String() < got[j].Chain.String()
		})
		require.Equal(t, want, got)

		//检查所有链必须已标记为停止
		for _, c := range want {
			number := GetStoppedChainFlag(db, c.Chain)
			require.Equal(t, c.Number, number)
		}
	}

	err := OnFoundBad(&dc, db, nil, badUnits, nil)
	require.NoError(t, err)
	got := GetStoppedChainByBad(db, badChainNumber.Chain, badChainNumber.Number)
	checkStopFlag(t, needStopped, got)

	//
	//                      B1<--------------B2<--------B3
	//						 ↓                ↓
	//      A1  <--------- (A2) <----------- A3<---------A4
	//		↑								   ↑
	//      C2<-------C3<-----------C4<------ C5<---------C6
	//                              ↓                     ↑
	//      D2<-------D3<-----------D4<------ D5<---------D6<-------D7
	//                                                    ↓
	//      E2<-------E3<-----------E4<------ E5<---------E6<-------E7
	//
	//当其他链出现作恶单元时，不能覆盖影响
	// 当单元D3标记为作恶时，最终的停摆标记应该是：A2,B1,C4,D3
	t.Run("moreBad", func(t *testing.T) {
		badUnits := []types.AffectedUnit{
			{
				Header: types.Header{MC: common.StringToAddress("D"), Number: 3},
				Votes: []types.WitenssVote{
					{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set(make([]byte, 50))},
				},
			},
			{
				Header: types.Header{MC: common.StringToAddress("D"), Number: 3, Timestamp: 1},
				Votes: []types.WitenssVote{
					{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set(make([]byte, 25))},
				},
			},
		}
		needStopped := []ChainNumber{
			{common.StringToAddress("C"), 4},
			{common.StringToAddress("D"), 3},
		}

		err := OnFoundBad(&dc, db, nil, badUnits, nil)
		require.NoError(t, err)
		got := GetStoppedChainByBad(db, common.StringToAddress("D"), 3)
		checkStopFlag(t, needStopped, got)

		//最终的停摆标记应该是：A2,B1,C4,D3
		needStopped = []ChainNumber{
			{common.StringToAddress("A"), 2},
			{common.StringToAddress("B"), 1},
			{common.StringToAddress("C"), 4},
			{common.StringToAddress("D"), 3},
		}
		//检查所有链
		for _, c := range needStopped {
			number := GetStoppedChainFlag(db, c.Chain)
			require.Equal(t, c.Number, number, "不能影响其他已停止的链")
		}

		//此时再恢复 D3 ，//最终的停摆标记应该是：A2,B1,C5,D6
		t.Run("recover", func(t *testing.T) {
			err := RecoverStartChain(&dc, db, common.StringToAddress("D"), 3)
			require.NoError(t, err, "必须能成功恢复")

			needStopped := []ChainNumber{
				{common.StringToAddress("A"), 2},
				{common.StringToAddress("B"), 1},
				{common.StringToAddress("C"), 5},
				{common.StringToAddress("D"), 6},
			}
			for _, c := range needStopped {
				number := GetStoppedChainFlag(db, c.Chain)
				require.Equal(t, c.Number, number, "不能影响其他已停止的链")
			}
		})
	})

	t.Run("recovery", func(t *testing.T) {
		err := RecoverStartChain(&dc, db, badChainNumber.Chain, badChainNumber.Number)
		require.NoError(t, err, "必须能成功恢复")
		for _, c := range chains {
			err := CheckChainIsStopped(db, c, 100)
			require.NoError(t, err, "所有链应该都已经恢复")
		}
	})

}
