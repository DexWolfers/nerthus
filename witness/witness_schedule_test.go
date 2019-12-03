package witness

import (
	"crypto/ecdsa"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/crypto"

	"math/big"

	"gitee.com/nerthus/nerthus/core/types"
	. "github.com/smartystreets/goconvey/convey"
)

func newTestBackend(t *testing.T, g *core.Genesis, witnessAccount common.Address, keys ...*ecdsa.PrivateKey) *TestBackendEx {
	ctx1, err := NewTestContext(g, keys...)
	if err != nil {
		t.Fatal(err)
	}
	ctx1.MainAccount = witnessAccount
	nts := TestBackendEx{ctx1}
	nts.Unlock(witnessAccount, "foobar")

	return &nts
}

func TestTaskSchedule(t *testing.T) {
	k1, _ := crypto.HexToECDSA("d49dcf37238dc8a7aac57dc61b9fee68f0a97f062968978b9fafa7d1033d03a9")
	witness := crypto.ForceParsePubKeyToAddress(k1.PublicKey)
	backend := newTestBackend(t, nil, witness, k1)

	Convey("task schedule start and stop", t, func() {
		witSchedule := NewWitnessSchedule(backend, nil)
		So(witSchedule, ShouldNotBeNil)
		Convey("Should be start success", func() {
			So(witSchedule.Start(), ShouldBeNil)
			So(witSchedule.isRunning(), ShouldBeTrue)
		})
		Convey("Should be stop success", func() {
			So(witSchedule.Stop(), ShouldBeNil)
			So(witSchedule.isRunning(), ShouldBeFalse)
		})
	})
	Convey("task schedule report event", t, func() {
		witSchedule := NewWitnessSchedule(backend, nil)
		So(witSchedule, ShouldNotBeNil)
		So(witSchedule.Start(), ShouldBeNil)
		waitWitScheduleStarted(witSchedule)
		time.Sleep(time.Second * 2)
		chain := witSchedule.nts.DagChain()
		So(chain, ShouldNotBeNil)
		Convey("receive a report event", func() {
			for i := 0; i < 100; i++ {
				reportUnitEvent := types.FoundBadWitnessEvent{
					Bad: *types.NewMockUnit(common.BigToAddress(big.NewInt(int64(i)))).Header(),
				}
				chain.PostChainEvents([]interface{}{reportUnitEvent}, nil)
			}
			time.Sleep(time.Millisecond * 100)

			for i := 0; i < 100; i++ {
				addr := common.BigToAddress(big.NewInt(int64(i)))
				ok := witSchedule.jobCenter.IsInBlack(addr)
				if !ok {
					t.Fatalf("expect true, got false")
				}
			}

			for i := 0; i < 100; i++ {
				reportUnitResultEvent := types.ReportUnitResultEvent{
					Unit: types.NewMockUnit(common.BigToAddress(big.NewInt(int64(i)))).Unit,
				}
				chain.PostChainEvents([]interface{}{reportUnitResultEvent}, nil)
			}
			time.Sleep(time.Millisecond * 100)

			for i := 0; i < 100; i++ {
				addr := common.BigToAddress(big.NewInt(int64(i)))
				ok := witSchedule.jobCenter.IsInBlack(addr)
				if ok {
					t.Fatalf("expect false, got true")
				}
			}
		})

	})
}

func waitWitScheduleStarted(schedule *WitnessSchedule) {
	for {
		if schedule.state == 1 {
			return
		}
	}
}
