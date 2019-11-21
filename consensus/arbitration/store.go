package arbitration

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/hashicorp/golang-lru"
)

var (
	keyBadWitnessProof = []byte("bad_witness_poof_")
	keyBadChain        = []byte("bad_chain_flags")
	keyStoppedChain    = []byte("bad_stopped_chain_")
	keyStoppedBy       = []byte("bad_stopped_by_")
	keyMyChoose        = []byte("bad_chain_witness_choose")
)

var chainStoppedCache *lru.Cache

func init() {
	chainStoppedCache, _ = lru.New(3000)
}

type ChainNumber struct {
	Chain  common.Address
	Number uint64
}

type StateDB interface {
	SetState(addr common.Address, key common.Hash, value common.Hash)
	GetState(a common.Address, b common.Hash) common.Hash
}

// 判断作恶证据是否已存储
func ExistProof(db ntsdb.Getter, id common.Hash) bool {
	key := append(keyBadWitnessProof, id.Bytes()...)
	v, _ := db.Get(key)
	return len(v) > 0
}

// 根据 ID 获取作恶数据
func GetBadWitnessProof(db ntsdb.Getter, id common.Hash) []types.AffectedUnit {
	b, _ := db.Get(append(keyBadWitnessProof, id.Bytes()...))
	if len(b) == 0 {
		return nil
	}
	var items []types.AffectedUnit
	if err := rlp.DecodeBytes(b, &items); err != nil {
		panic(err)
	}
	return items
}

// 存储作恶证据
func writeBadWitneeProof(db ntsdb.PutGetter, rep []types.AffectedUnit) (id common.Hash) {
	b, err := rlp.EncodeToBytes(rep)
	if err != nil {
		panic(err)
	}

	id = common.SafeHash256(rep)
	//创建 DB
	key := append(keyBadWitnessProof, id.Bytes()...)
	db.Put(key, b)

	newFlag := BadChainFlag{Chain: rep[0].Header.MC, Number: rep[0].Header.Number, ProofID: id}
	UpdateBadChainFlag(db, newFlag)
	return id
}

// 坏单元标记
type BadChainFlag struct {
	Chain          common.Address //作恶链
	Number         uint64         //作恶高度
	FoundTimestamp uint64         //本地作恶发现时间
	ProofID        common.Hash    //作恶证据存储 ID
}

// 更新坏链标记，已最小高度为准。
// 当更换成功时返回 TRUE
// 注意：这里尚未更新所有，考虑的是一条链在一个时刻只可能出现一个作恶位置。
// 这是因为一旦作恶发送这条链则依旧停止工作，不允许有新单元进入。
// 但一种情况是，先发现高高度作恶，再发现低高度作恶，此时便会出现覆盖。
// 优先解决低高度作恶
func UpdateBadChainFlag(db ntsdb.PutGetter, newFlag BadChainFlag) {
	newFlag.FoundTimestamp = uint64(time.Now().Unix())
	writeBadChain(db, newFlag)
}

// 获取所有处于 Bad 情况下的链
func GetAllBadChain(db ntsdb.Finder) []common.Address {
	var list []common.Address
	db.Iterator(keyBadChain, func(key, value []byte) bool {
		list = append(list, common.BytesToAddress(key[len(keyBadChain):]))
		return true
	})
	return list
}

// 获取坏链标记
func GetBadChainFlag(db ntsdb.Getter, chain common.Address) (badChainFlag BadChainFlag) {
	flags := GetBadChainFlags(db, chain)
	if len(flags) > 0 {
		badChainFlag = flags[0]
	}
	return
}
func clearBadChainFlag(db ntsdb.PutGetter, chain common.Address, number uint64) bool {
	flags := GetBadChainFlags(db, chain)
	var find bool
	for i := 0; i < len(flags); i++ {
		if flags[i].Number == number {
			//说明存在，删除此标记
			if i == len(flags)-1 {
				flags = flags[:i]
			} else {
				flags = append(flags[:i], flags[i+1:]...)
			}
			find = true
			break
		}
	}
	if find {
		b, err := rlp.EncodeToBytes(flags)
		if err != nil {
			panic(err)
		}
		db.Put(append(keyBadChain, chain.Bytes()...), b)

	}
	return len(flags) == 0
}

// 获取坏链标记
func GetBadChainFlags(db ntsdb.Getter, chain common.Address) []BadChainFlag {
	b, _ := db.Get(append(keyBadChain, chain.Bytes()...))
	if len(b) == 0 {
		return nil
	}

	var flags []BadChainFlag
	if err := rlp.DecodeBytes(b, &flags); err != nil {
		panic(err)
	}
	sort.Slice(flags, func(i, j int) bool {
		return flags[i].Number < flags[j].Number
	})
	return flags
}
func writeBadChain(db ntsdb.PutGetter, badChainFlag BadChainFlag) {
	flags := GetBadChainFlags(db, badChainFlag.Chain)
	for _, v := range flags {
		if v.Number == badChainFlag.Number { //不重复处理
			return
		}
	}
	flags = append(flags, badChainFlag)
	//记录有Bad链
	b, err := rlp.EncodeToBytes(flags)
	if err != nil {
		panic(err)
	}
	badKey := append(keyBadChain, badChainFlag.Chain.Bytes()...)
	db.Put(badKey, b)
}

// 判断链在此高度是否已经停止
func CheckChainIsStopped(db ntsdb.Getter, chain common.Address, number uint64) error {
	if chain == params.SCAccount { //系统链特殊不进行检查
		return nil
	}
	stoppedNumber := GetStoppedChainFlag(db, chain)
	if stoppedNumber > 0 && stoppedNumber <= number {
		return fmt.Errorf("chain has stopped:%s(%d)", chain.Text(), stoppedNumber)
	}
	return nil
}

// 获取因为坏单元而引起被停止的相关链
func GetStoppedChainByBad(db ntsdb.Getter, chain common.Address, number uint64) []ChainNumber {
	var stopped []ChainNumber

	b, _ := db.Get(append(keyStoppedBy, append(chain.Bytes(), common.Uint64ToBytes(number)...)...))
	if len(b) == 0 {
		return nil
	}
	rlp.DecodeBytes(b, &stopped)
	return stopped
}

// 登记因为作恶单元引起相关链被暂停的关系，以便通过此进行恢复
func writeStoppedByWho(db ntsdb.PutGetter, chain common.Address, number uint64, stoppedChains map[common.Address]uint64) {
	if len(stoppedChains) == 0 {
		return
	}
	stopped := make([]ChainNumber, 0, len(stoppedChains))
	for c, n := range stoppedChains {
		stopped = append(stopped, ChainNumber{c, n})
	}

	b, err := rlp.EncodeToBytes(stopped)
	if err != nil {
		panic(err)
	}
	db.Put(append(keyStoppedBy, append(chain.Bytes(), common.Uint64ToBytes(number)...)...), b)
}

// 清理停摆标记，并返还是否该链是否已经无停摆标记
func clearStoppedChainFlag(db ntsdb.PutGetter, chain common.Address, number uint64) bool {
	numbers := getStoppedChainFlag(db, chain)
	var find bool
	for i := 0; i < len(numbers); i++ {
		if numbers[i] == number {
			//说明存在，删除此标记
			if i == len(numbers)-1 {
				numbers = numbers[:i]
			} else {
				numbers = append(numbers[:i], numbers[i+1:]...)
			}
			find = true
			break
		}
	}
	if find {
		b, err := rlp.EncodeToBytes(numbers)
		if err != nil {
			panic(err)
		}
		db.Put(append(keyStoppedChain, chain.Bytes()...), b)

		chainStoppedCache.Remove(chain)
	}
	return len(numbers) == 0
}
func GetStoppedChainFlag(db ntsdb.Getter, chain common.Address) (number uint64) {
	item, ok := chainStoppedCache.Get(chain)
	if ok {
		return item.(uint64)
	}

	items := getStoppedChainFlag(db, chain)
	if len(items) > 0 {
		number = items[0]
	}
	chainStoppedCache.Add(chain, number)

	return number
}
func getStoppedChainFlag(db ntsdb.Getter, chain common.Address) []uint64 {
	b, _ := db.Get(append(keyStoppedChain, chain.Bytes()...))
	if len(b) == 0 {
		return nil
	}
	var numbers []uint64
	if err := rlp.DecodeBytes(b, &numbers); err != nil {
		panic(err)
	}
	sort.Slice(numbers, func(i, j int) bool {
		return numbers[i] < numbers[j]
	})
	return numbers
}

// 记录链停止信息
func writeStoppedChain(db ntsdb.PutGetter, chain common.Address, number uint64) bool {
	//只需要
	stoppedNumber := GetStoppedChainFlag(db, chain)
	// 如果旧标记高度晚于新标记，则需要更新，以便处理旧记录
	if stoppedNumber == 0 || stoppedNumber > number {
		chainStoppedCache.Add(chain, number)
	}
	items := getStoppedChainFlag(db, chain)
	//去重
	for _, v := range items {
		if v == number {
			return false
		}
	}

	items = append(items, number)
	b, err := rlp.EncodeToBytes(items)
	if err != nil {
		panic(err)
	}
	db.Put(append(keyStoppedChain, chain.Bytes()...), b)

	return true
}
func getMyChooseVoteTx(db ntsdb.Getter, chain common.Address, number uint64) *types.Transaction {
	b, _ := db.Get(append(keyMyChoose, append(chain.Bytes(), common.Uint64ToBytes(number)...)...))
	if len(b) == 0 {
		return nil
	}
	var tx types.Transaction
	if err := rlp.DecodeBytes(b, &tx); err != nil {
		panic(err)
	}
	return &tx

}
func writeMyChooseVoteTx(db ntsdb.PutGetter, chain common.Address, number uint64, tx *types.Transaction) {
	b, err := rlp.EncodeToBytes(tx)
	if err != nil {
		panic(err)
	}
	db.Put(append(keyMyChoose, append(chain.Bytes(), common.Uint64ToBytes(number)...)...), b)
}
func deleteMyChooseVoteTx(db ntsdb.PutGetter, chain common.Address, number uint64) {
	db.Put(append(keyMyChoose, append(chain.Bytes(), common.Uint64ToBytes(number)...)...), []byte{})
}

// 对集合数据进行排序
func SortProof(rep []types.AffectedUnit) {
	for i := 0; i < len(rep); i++ {
		sort.Slice(rep[i].Votes, func(ii, jj int) bool {
			return bytes.Compare(rep[i].Votes[ii].Sign.Get(), rep[i].Votes[jj].Sign.Get()) < 0
		})
	}
	//按时间排序，已方便大概率产生相同的 RLP Hash 值。降低重复处理频次
	sort.Slice(rep, func(i, j int) bool {
		return rep[i].Header.Timestamp < rep[j].Header.Timestamp
	})
}
