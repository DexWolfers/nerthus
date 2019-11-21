package core

import (
	"math/big"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
)

/*
将热点链的尾部信息缓存，降低IO影响
*/

// ChainCache 链信息缓存
type ChainCache struct {
	chain     common.Address
	balance   *big.Int
	tailHash  common.Hash
	tailUnit  *types.Unit
	tailState *state.StateDB

	lock           sync.RWMutex
	isCacheStateDB bool
	dagchain       *DagChain
}

// 创建一个链缓存，将自动显示链最新状态数据，包括余额
func NewChainCache(chain common.Address, sdb state.Database, uhash common.Hash, unit *types.Unit) (*ChainCache, error) {
	tailState, err := state.New(unit.MC().Bytes(), unit.Root(), sdb)
	if err != nil {
		return nil, err
	}
	c := ChainCache{
		chain:     chain,
		balance:   tailState.GetBalance(chain),
		tailHash:  uhash,
		tailState: tailState,
		tailUnit:  unit,
	}
	return &c, nil
}

func NewChainCacheWithState(chain common.Address, tailState *state.StateDB, unit *types.Unit) *ChainCache {
	c := ChainCache{
		chain:     chain,
		balance:   tailState.GetBalance(chain),
		tailHash:  unit.Hash(),
		tailState: tailState,
		tailUnit:  unit,
	}
	return &c
}
func (c *ChainCache) SwitchCacheUnit(on bool, dag *DagChain) {
	c.isCacheStateDB = on
	if !on {
		c.tailUnit = nil
		c.tailState = nil
		c.dagchain = dag
	}
}

// UpTail 更新链尾部信息：
func (c *ChainCache) UpTail(tailState *state.StateDB, uhash common.Hash, u *types.Unit) {
	b := tailState.GetBalance(c.chain)
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.isCacheStateDB {
		c.tailState = tailState
		c.tailUnit = u
	}
	c.balance = b
	c.tailHash = uhash
}

func (c *ChainCache) Balance() *big.Int {
	c.lock.RLock()
	b := c.balance
	c.lock.RUnlock()
	return b
}

func (c *ChainCache) TailHash() common.Hash {
	c.lock.RLock()
	h := c.tailHash
	c.lock.RUnlock()
	return h
}
func (c *ChainCache) TailHead() *types.Header {
	c.lock.RLock()
	if c.tailUnit != nil {
		c.lock.RUnlock()
		return c.tailUnit.Header()
	}
	c.lock.RUnlock()
	return c.dagchain.GetHeaderByHash(c.tailHash)
}
func (c *ChainCache) TailUnit() *types.Unit {
	c.lock.RLock()
	u := c.tailUnit
	c.lock.RUnlock()
	if u == nil {
		u = c.dagchain.GetUnitByHash(c.tailHash)
	}
	return u
}
func (c *ChainCache) State(sdb state.Database) *state.StateDB {
	if c.isCacheStateDB && c.tailState != nil {
		c.lock.RLock()
		var s *state.StateDB
		if c.tailState != nil {
			s = c.tailState.Copy()
		}
		c.lock.RUnlock()
		if s != nil {
			return s
		}
	}
	s, _ := state.New(c.TailUnit().MC().Bytes(), c.TailUnit().Root(), sdb)
	return s
}

func (c *ChainCache) CopyStateTrie() *state.StateDB {
	c.lock.RLock()
	s := c.tailState.NewCopyTrie()
	c.lock.RUnlock()
	return s
}
