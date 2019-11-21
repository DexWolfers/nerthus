package bft

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
)

func NewDefValidator(addr common.Address) Validator {
	return &defaultValidator{address: addr}
}

type defaultValidator struct {
	address common.Address
}

func NewValidator(address common.Address) Validator {
	return &defaultValidator{address: address}
}

func (val *defaultValidator) Address() common.Address {
	if val == nil {
		return common.EmptyAddress
	}
	return val.address
}

func (val *defaultValidator) String() string {
	return val.address.String()
}

type Validators []Validator

func ValidatorsSort(validators Validators) {
	sort.SliceStable(validators, func(i, j int) bool {
		vx, vy := validators[i].(Validator), validators[j].(Validator)
		return strings.Compare(vx.String(), vy.String()) < 0
	})
}

func (vals Validators) Size() int {
	return len(vals)
}

func (vals Validators) String() string {
	var strs = make([]string, len(vals))
	for i, val := range vals {
		strs[i] = val.String()[:10]
	}
	return strings.Join(strs, ",")
}

func NewSet(blcHeight uint64, preHash common.Hash, validators Validators, chain common.Address) ValidatorSet {
	return newDefaultSet(blcHeight, preHash, validators, chain)
}

func newDefaultSet(blcHeight uint64, preHash common.Hash, validators Validators, chain common.Address) *defaultSet {
	defSet := &defaultSet{}
	defSet.selector = nextRoundProposer
	defSet.rwLock = new(sync.RWMutex)
	defSet.height = blcHeight
	defSet.preHash = preHash
	defSet.chain = chain
	// 计算偏移量
	defSet.solt = fnv32(chain.Hex())
	defSet.validators = validators

	ValidatorsSort(defSet.validators)
	// NOTE init proposer
	if defSet.Size() > 0 {
		defSet.proposer = defSet.GetByIndex(0)
	}
	return defSet
}

func fnv32(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

type defaultSet struct {
	height     uint64
	preHash    common.Hash
	validators Validators
	proposer   Validator
	rwLock     *sync.RWMutex
	selector   ProposalSelector
	chain      common.Address
	solt       uint32
}

func (defSet *defaultSet) Size() (s int) {
	defSet.rwLock.RLock()
	s = defSet.validators.Size()
	defSet.rwLock.RUnlock()
	return
}

func (defSet *defaultSet) String() string {
	return fmt.Sprintf("height:%v, preHash:%v, proposer:%v, vals:[%v]", defSet.height, defSet.preHash.TerminalString(), defSet.proposer.String()[:10], defSet.validators.String())
}

func (defSet *defaultSet) GetByIndex(idx uint64) (val Validator) {
	defSet.rwLock.RLock()
	val = defSet.validators[idx]
	defSet.rwLock.RUnlock()
	return
}

func (defSet *defaultSet) GetByAddress(addr common.Address) (int, Validator) {
	valList := defSet.List()
	for i, val := range valList {
		if val.Address() == addr {
			return i, val
		}
	}
	return -1, nil
}

func (defSet *defaultSet) GetProposer() Validator {
	return defSet.proposer
}

func (defSet *defaultSet) IsProposer(address common.Address) bool {
	valAddr := defSet.GetProposer().Address()
	return (valAddr.Empty() || address.Empty()) == false && (valAddr == address)
}
func (defSet *defaultSet) IsNextProposer(address common.Address, round uint64) bool {
	return address == nextRoundProposer(defSet.validators, defSet.proposer.Address(), round, defSet.height, defSet.preHash).Address()
}

// return mut validator
// NOTE: don't change valList item Value
func (defSet *defaultSet) List() (valList []Validator) {
	defSet.rwLock.RLock()
	valList = defSet.validators
	defSet.rwLock.RUnlock()
	return
}

// 计算next round的提案者
func (defSet *defaultSet) CalcProposer(proposer common.Address, round uint64) {
	defSet.rwLock.Lock()
	defSet.proposer = defSet.selector(defSet.validators, proposer, round, defSet.height+uint64(defSet.solt), defSet.preHash)
	defSet.rwLock.Unlock()
}

func (defSet *defaultSet) AddValidator(address common.Address) bool {
	defSet.rwLock.Lock()
	for _, v := range defSet.validators {
		if v.Address() == address {
			defSet.rwLock.Unlock()
			return false
		}
	}
	defSet.validators = append(defSet.validators, &defaultValidator{address: address})
	ValidatorsSort(defSet.validators)
	defSet.rwLock.Unlock()
	return true
}

func (defSet *defaultSet) RemoveValidator(address common.Address) bool {
	defSet.rwLock.Lock()
	for i, v := range defSet.validators {
		if v.Address() == address {
			defSet.validators = append(defSet.validators[:i], defSet.validators[i+1:]...)
			defSet.rwLock.Unlock()
			return true
		}
	}
	defSet.rwLock.Unlock()
	return false
}

func (defSet *defaultSet) Copy() ValidatorSet {
	defSet.rwLock.RLock()
	addresses := make([]common.Address, 0, defSet.validators.Size())
	for _, v := range defSet.validators {
		addresses = append(addresses, v.Address())
	}
	defSet.rwLock.RUnlock()
	return newDefaultSet(defSet.height, defSet.preHash, defSet.validators, defSet.chain)
}

func (delSet *defaultSet) F() int {
	return delSet.Size() / 3
}

func (delSet *defaultSet) TwoThirdsMajority() int {
	if delSet.chain == params.SCAccount {
		return int(params.ConfigParamsSCMinVotings - 1)
	}
	return int(params.ConfigParamsUCMinVotings - 1)
}

// TODO use preHash as seed
// proposer 为preProposer(上一区块或者空块)
func nextRoundProposer(vals Validators, proposer common.Address, round uint64, height uint64, preHash ...common.Hash) Validator {
	if vals == nil {
		panic("validators set is nil")
	}
	seed := uint64(0)
	if proposer.Empty() {
		seed = height + round
	} else {
		seed = calcSeed(vals, proposer, round) + 1
	}
	pick := seed % uint64(vals.Size())
	return vals[pick]
}

func calcSeed(vals Validators, proposer common.Address, round uint64) uint64 {
	offset := 0
	for i, val := range vals {
		if val.Address() == proposer {
			offset = i
			break
		}
	}
	return uint64(offset) + round
}
