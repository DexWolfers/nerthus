package trace

import (
	"fmt"
	"math/big"
	"runtime/debug"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/ntsdb"

	"github.com/pkg/errors"
)

type PrivateTraceAPI struct {
	db      ntsdb.Database
	backend *core.DagChain
}

func NewPrivateTraceAPI(dc *core.DagChain, db ntsdb.Database) *PrivateTraceAPI {
	return &PrivateTraceAPI{
		db:      db,
		backend: dc,
	}
}

func (t *PrivateTraceAPI) ExecUnit(uhash common.Hash) (types.Receipts, error) {
	unit := t.backend.GetUnit(uhash)
	if unit == nil {
		return nil, errors.New("not found unit by hash")
	}

	var (
		totalUsed = big.NewInt(0)
		results   types.Receipts
	)

	state, err := t.backend.GetUnitState(unit.ParentHash())
	if err != nil {
		return nil, err
	}

	gp := core.GasPool(*new(big.Int).SetUint64(unit.GasLimit()))
	header := unit.Header()
	for i, tx := range unit.Transactions() {
		//执行
		//删除检查
		action := tx.Action
		if action == types.ActionSCIdleContractDeal {
			action = types.ActionContractDeal
		}
		state.SetState(unit.MC(), core.GetTxActionStoreKey(tx.TxHash, action), common.Hash{})

		r, err := core.ProcessTransaction(t.backend.Config(), t.backend, state, header, &gp, tx, totalUsed, vm.Config{Debug: true}, nil)
		if err != nil {
			return nil, fmt.Errorf("%d:tx is %s,action is %s,%v", i, tx.TxHash.String(), action.String(), err)
		}
		results = append(results, r)
	}
	return results, nil
}

func (t *PrivateTraceAPI) ExecTransaction(txhash common.Hash) (types.Receipts, error) {
	tx := t.backend.GetTransaction(txhash)
	if tx == nil {
		return nil, errors.New("not found transaction in unit")
	}

	var results types.Receipts

	err := core.GetTransactionStatus(t.db, txhash, func(info core.TxStatusInfo) (ok bool, err error) {

		defer func() {
			if err2 := recover(); err2 != nil {
				err = fmt.Errorf("%s", err2)
				debug.PrintStack()
			}
		}()

		unit := t.backend.GetUnit(info.UnitHash)
		if unit == nil {
			return false, errors.New("not found unit by hash")
		}

		var (
			stateRoot = unit.Root()
			receipts  = core.GetUnitReceipts(t.db, unit.MC(), unit.Hash(), unit.Number())
			index     = info.TxExecIndex - 1
			totalUsed = big.NewInt(0)
		)
		if index > 0 {
			//stateRoot = receipts[index-1].PostState
			totalUsed.SetUint64(receipts[index-1].CumulativeGasUsed.Uint64())
		}

		state, err := t.backend.StateAt(unit.MC(), stateRoot)
		if err != nil {
			return false, err
		}

		gp := core.GasPool(*new(big.Int).SetUint64(unit.GasLimit()))
		gp.SubGas(totalUsed.Uint64())

		//删除检查
		state.SetState(unit.MC(), core.GetTxActionStoreKey(unit.Transactions()[index].TxHash, unit.Transactions()[index].Action), common.Hash{})

		//执行
		r, err := core.ProcessTransaction(t.backend.Config(), t.backend, state, unit.Header(), &gp, unit.Transactions()[index], totalUsed, vm.Config{Debug: true}, nil)
		if err != nil {
			return false, err
		}
		results = append(results, r)

		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
