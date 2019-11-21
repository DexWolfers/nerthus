package clean

import (
	"errors"
	"fmt"
	"gitee.com/nerthus/nerthus/params"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/core/rawdb"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
)

func Recover(dagChain *core.DagChain, db ntsdb.Database, chain common.Address, from, to uint64) error {
	// 获取清理的截止时间
	k := rawdb.CleanerKey(chain)
	data, err := db.Get(k)
	if err == nil {
		lastCleanNumber := big.NewInt(0).SetBytes(data).Uint64()
		if to > lastCleanNumber {
			to = lastCleanNumber
		}
	} else {
		log.Error("failed to get last clean number", "err", err)
	}

	genesis := dagChain.GetHeaderByNumber(chain, from)
	if genesis == nil {
		return errors.New("failed to get genesis")
	}
	state, err := dagChain.GetStateDB(genesis.MC, genesis.StateRoot)
	if err != nil {
		return err
	}
	if genesis.Number == 0 && chain != params.SCAccount {
		state, err = state.NewChainState(chain)
		if err != nil {
			return err
		}
	}
	now := time.Now()
	for i := uint64(from + 1); i < to; i++ {
		u := dagChain.GetUnitByNumber(chain, i)
		if next, err := recoverOne(u, state, dagChain, db); err != nil {
			return err
		} else {
			state = next
		}
	}
	// 清理标识
	db.Delete(k)
	log.Info("recover chain state data", "chain", chain, "to", to, "cost", time.Now().Sub(now))
	return nil

}
func recoverOne(unit *types.Unit, state *state.StateDB, dagChain *core.DagChain, db ntsdb.Database) (*state.StateDB, error) {
	state = state.Copy()
	_, _, _, err := dagChain.Processor().Process(unit, state, dagChain.VMConfig())
	root, err := state.CommitToChain(db, unit.MC())
	log.Debug("recover one number state", "number", unit.Number(), "hash", unit.Hash(), "root", root, "err", err)
	return state, err
}

func FindAllKeys(dagChain *core.DagChain, db ntsdb.Database, hash common.Hash) map[common.Hash]int {
	cacheKeys := make(map[common.Hash]int)
	unit := dagChain.GetHeaderByHash(hash)
	rangeKey(db, unit.MC, unit.StateRoot.Bytes(), cacheKeys)
	return cacheKeys
}

func rangeKey(db ntsdb.Database, chain common.Address, root []byte, cacheKeys map[common.Hash]int) {
	state.RangeTrieKey(db, chain, root, func(key []byte) {
		hk := common.BytesToHash(key)
		if i, ok := cacheKeys[hk]; ok {
			cacheKeys[hk] = i + 1
			if i == 0 {
				fmt.Printf("%x\n", hk)
			}
		} else {
			cacheKeys[hk] = 0
		}
	})
}
