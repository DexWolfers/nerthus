package sc

import (
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/ntsdb"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
)

func TestGetPeriodRange(t *testing.T) {
	oneP := uint64(params.ConfigParamsBillingPeriod)
	caces := []struct {
		p, wantNumber uint64
	}{
		{1, oneP},
		{2, oneP * 2},
		{4, oneP * 4},
	}
	for _, c := range caces {
		_, got := GetPeriodRange(c.p)
		require.Equal(t, c.wantNumber, got, "call(%d)", c.p)

	}
	for i := 0; i < 11; i++ {
		t.Log(GetPeriodRange(uint64(i)))
	}
}

func TestCheckSettlementPeriod(t *testing.T) {

	caces := []struct {
		p, now, want uint64
	}{
		{1, 5, 1},
		{1, params.MaxSettlementPeriod, 1},
		{1, params.MaxSettlementPeriod * 2, params.MaxSettlementPeriod},
		{params.MaxSettlementPeriod*2 - 1, params.MaxSettlementPeriod * 2, params.MaxSettlementPeriod*2 - 1},
		{params.MaxSettlementPeriod*2 - 1, params.MaxSettlementPeriod, params.MaxSettlementPeriod*2 - 1},
	}
	for _, c := range caces {
		got := checkSettlementPeriod(c.p, c.now)
		require.Equal(t, c.want, got, "call(%d,%d)", c.p, c.now)
	}
}

func TestGetFreezingWitnessFee(t *testing.T) {
	db := newTestState()

	witness := common.StringToAddress("a")

	defer func() {
		getPeriodWorkFeeTest = nil
	}()

	t.Run("first", func(t *testing.T) {
		number := uint64(params.ConfigParamsBillingPeriod)*2 + 1
		currentPeriod := GetSettlePeriod(number)

		//应该从周期到 currentPeriod 的倒数1个周期
		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			p := fromPeriod.Uint64()
			require.True(t, p == currentPeriod || p == currentPeriod-1)
			return 100
		}

		freezing, err := GetFreezingWitnessFee(db, nil, witness, number)
		require.NoError(t, err)
		require.Equal(t, uint64(2*100), freezing.Uint64())
	})

	t.Run("isdone", func(t *testing.T) {

		// 测试场景：当前周期是第三周，下次提现周期从第二周开始。
		//  因此冻结部分计算：应该有：第三周和第二周。
		number := uint64(params.ConfigParamsBillingPeriod)*2 + 1
		currentPeriod := GetSettlePeriod(number)
		setNextWithdrawPeriod(db, witness, currentPeriod-1)
		//假如取款了currentPeriod-1，则冻结部分只能是：currentPeriod

		//应该从周期到 currentPeriod 的倒数1个周期
		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			p := fromPeriod.Uint64()
			require.True(t, p == currentPeriod || p == currentPeriod-1)
			return 100
		}

		freezing, err := GetFreezingWitnessFee(db, nil, witness, number)
		require.NoError(t, err)
		require.Equal(t, uint64(100*2), freezing.Uint64())
	})

	t.Run("allDone", func(t *testing.T) {
		// 测试场景： 当前周期是第三周，下次提现周期从第三周开始。
		//  因此冻结部分计算：应该只有第三周
		number := uint64(params.ConfigParamsBillingPeriod)*2 + 1
		currentPeriod := GetSettlePeriod(number)
		setNextWithdrawPeriod(db, witness, currentPeriod)
		//假如取款了currentPeriod-1，则冻结部分只能是：currentPeriod

		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			p := fromPeriod.Uint64()
			require.Equal(t, currentPeriod, p)
			return 100
		}

		freezing, err := GetFreezingWitnessFee(db, nil, witness, number)
		require.NoError(t, err)
		require.Equal(t, uint64(100), freezing.Uint64())
	})

	t.Run("all", func(t *testing.T) {
		// 测试场景： 当前周期是第三周，下次提现周期从第四周开始。
		//  因此冻结部分计算：无
		number := uint64(params.ConfigParamsBillingPeriod)*2 + 1
		currentPeriod := GetSettlePeriod(number)
		setNextWithdrawPeriod(db, witness, currentPeriod+1)

		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Fail(t, "should be not call")
			return 0
		}

		freezing, err := GetFreezingWitnessFee(db, nil, witness, number)
		require.NoError(t, err)
		require.Zero(t, freezing.Uint64())
	})
}

func TestGetWithdrawableWitnessFee(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	sdb := newTestState()
	defer func() {
		getPeriodWorkFeeTest = nil
	}()

	witness := common.StringToAddress("a")

	t.Run("first", func(t *testing.T) {
		number := uint64(params.ConfigParamsBillingPeriod)*5 + 1 //相当于6个周期
		//currentPeriod:= GetSettlePeriod(sdb, number)

		setNextWithdrawPeriod(sdb, witness, 1) //设置为1
		//计算可取款应该是从：[下次取款位置，到本次周期-1] (本次周期=6)
		//因为设置了下次取款开始周期是1，则本次计算可取款周期范围是： [1,6-1）----> [1,4]

		needCalcPeriod := map[uint64]struct{}{
			uint64(1): struct{}{},
			uint64(2): struct{}{},
			uint64(3): struct{}{},
			uint64(4): struct{}{},
		}
		amount := uint64(1000)
		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			require.Contains(t, needCalcPeriod, fromPeriod.Uint64())
			//只能计算一次，删除
			delete(needCalcPeriod, fromPeriod.Uint64())
			return amount
		}

		freezing, nextStart, err := GetWithdrawableWitnessFee(sdb, db, witness, number)
		require.NoError(t, err)
		//下一次取款应该从5开始
		require.Equal(t, uint64(5), nextStart)
		// 提现是四个周期汇总
		require.Equal(t, amount*4, freezing.Uint64())
	})

	t.Run("second", func(t *testing.T) {
		number := uint64(params.ConfigParamsBillingPeriod)*5 + 1 //相当于6个周期
		//currentPeriod:= GetSettlePeriod(sdb, number)

		setNextWithdrawPeriod(sdb, witness, 4) //设置为4
		//计算可取款应该是从：[下次取款位置，到本次周期-1] (本次周期=6)
		//因为设置了下次取款开始周期是1，则本次计算可取款周期范围是： [4,6-1）----> [4,4]

		needCalcPeriod := map[uint64]struct{}{
			uint64(4): struct{}{},
		}
		amount := uint64(1000)
		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			require.Contains(t, needCalcPeriod, fromPeriod.Uint64())
			//只能计算一次，删除
			delete(needCalcPeriod, fromPeriod.Uint64())
			return amount
		}

		freezing, nextStart, err := GetWithdrawableWitnessFee(sdb, db, witness, number)
		require.NoError(t, err)
		//下一次取款应该从5开始
		require.Equal(t, uint64(5), nextStart)
		// 提现是一个周期汇总
		require.Equal(t, amount, freezing.Uint64())
	})

	t.Run("zero", func(t *testing.T) {
		number := uint64(params.ConfigParamsBillingPeriod)*5 + 1 //相当于6个周期
		//currentPeriod:= GetSettlePeriod(sdb, number)

		setNextWithdrawPeriod(sdb, witness, 5) //设置为4
		//计算可取款应该是从：[下次取款位置，到本次周期-1] (本次周期=6)
		//因为设置了下次取款开始周期是1，则本次计算可取款周期范围是： [5,6-1）----> [5,4] --> 0

		needCalcPeriod := map[uint64]struct{}{}
		amount := uint64(1000)
		getPeriodWorkFeeTest = func(addr common.Address, fromPeriod *big.Int) uint64 {
			require.Equal(t, witness, addr)
			require.Contains(t, needCalcPeriod, fromPeriod.Uint64())
			return amount
		}

		freezing, nextStart, err := GetWithdrawableWitnessFee(sdb, db, witness, number)
		require.NoError(t, err)
		//下一次取款应该从还是从5开始
		require.Equal(t, uint64(5), nextStart)
		// 应该不一个周期可取
		require.Zero(t, freezing.Uint64())
	})

}

func TestGetWitnessPeriodWorkReports(t *testing.T) {

	dbf, err := ioutil.TempDir("", "nerthustest")
	require.NoError(t, err)
	defer os.RemoveAll(dbf)

	db, err := ntsdb.NewLDBDatabase(dbf, 1, 1)
	require.NoError(t, err)
	defer db.Close()

	//写入不同周期数据，最终能根据前缀遍历所有记录
	for period := uint64(1); period < 4; period++ {
		items := GetWitnessPeriodWorkReports(db, period)
		require.Empty(t, items)

		want := map[common.Address]types.WitnessPeriodWorkInfo{
			common.StringToAddress("w1"): {
				Votes:         1 + period,
				CreateUnits:   2,
				VotedTxs:      3,
				ShouldVote:    4,
				Fee:           math.MaxBig256,
				VotedLastUnit: common.StringToHash("u1"),
			},
			common.StringToAddress("w2"): {
				Votes:         11 + period,
				CreateUnits:   21,
				VotedTxs:      31,
				ShouldVote:    41,
				Fee:           math.MaxBig63,
				VotedLastUnit: common.StringToHash("u2"),
			},
			common.StringToAddress("w3"): {
				Votes:         12 + period,
				CreateUnits:   22,
				VotedTxs:      32,
				ShouldVote:    42,
				Fee:           big.NewInt(1),
				VotedLastUnit: common.StringToHash("u3"),
			},
		}

		for w, info := range want {
			key := getSettlementPeriodLogKey(period, w)
			updateWitnessWorkReport(db, key, &info)
		}
		got := GetWitnessPeriodWorkReports(db, period)
		require.Len(t, got, len(want))
		for w, info := range want {
			gotInfo, ok := got[w]
			require.True(t, ok)
			require.Equal(t, info, gotInfo)
		}
	}
}
