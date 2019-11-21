package ethash

import (
	"encoding/binary"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/params"
)

type VerifyNoFeeTxHandle = func(tx *types.Transaction) bool

// 1、to为合约地址，seed不能为nil，diff需要校验
// 2、具体的业务需要手动处理proof work以外VerifyNoFeeTXHandle,
// 	如：委托系统见证人更换"见证人"，限制该交易必须为用户链的第一笔交易
//func IsNoFeeTx(tx *types.Transaction, verifyNoFeeTxHandles ...VerifyNoFeeTxHandle) bool {
//	var to, abiFn = tx.To(), tx.Data()
//	var noFeeTxs = params.NoFeeTx()
//	if to.Empty() || !to.IsContract() || len(abiFn) == 0 || tx.Seed() == nil {
//		log.Debug("IsNoFeeTx", "to", to, "abiFn", abiFn, "seed", tx.Seed(), "pass", false)
//		return false
//	}
//
//	if abiFns, ok := noFeeTxs[to.Hex()]; ok {
//		abiFnNum := common.BytesToHash(abiFn).Big().Uint64()
//		diff, ok := abiFns[abiFnNum]
//		log.Debug("IsNoFeeTx", "tx abiFnNum", abiFnNum, "ok", ok, "diff", diff)
//		if !ok || !VerifyProofWork(tx, diff) {
//			log.Debug("IsNoFeeTx", "abiFnNum", abiFnNum, "ok", ok, "seed", tx.Seed().Uint64(), "diff", diff, "pass", false)
//			return false
//		}
//	}
//
//	for _, handle := range verifyNoFeeTxHandles {
//		if !handle(tx) {
//			log.Debug("IsNoFeeTx", "handle", handle, "pass", false)
//			return false
//		}
//	}
//	log.Debug("IsNoFeeTx", "txHash", tx.Hash().Hex(), "to", tx.To().Hex(), "seed", tx.Seed().Uint64())
//	return true
//}

// ProofWork miner search a seed from seed and recommend seed = 1
func ProofWork(tx *types.Transaction, seed uint64, diff *big.Int, abort <-chan struct{}) <-chan uint64 {
	var (
		hash   = tx.HashNoSeed()
		seedCh = make(chan uint64, 1)
	)
	target := big.NewInt(0).Sub(params.MaxUint256, common.Big1)
	target = big.NewInt(0).Rsh(target, uint(diff.Uint64()))

	go func() {
		defer close(seedCh)
		hashTxt := make([]byte, common.HashLength+8)
		copy(hashTxt[:common.HashLength], hash.Bytes())
		for {
			select {
			case <-abort:
				return
			default:
				binary.LittleEndian.PutUint64(hashTxt[common.HashLength:], seed)
				digest := crypto.Keccak256(hashTxt[:])
				if new(big.Int).SetBytes(digest[:]).Cmp(target) <= 0 {
					seedCh <- seed
					return
				}
			}
			seed++
		}
	}()
	return seedCh
}

// VerifyProofWork check transaction pprof work
func VerifyProofWork(tx *types.Transaction, diff *big.Int) bool {

	if tx.Seed() == 0 {
		return false
	}
	var (
		hash = tx.HashNoSeed()
	)
	target := big.NewInt(0).Sub(params.MaxUint256, common.Big1)
	target = big.NewInt(0).Rsh(target, uint(diff.Uint64()))
	hashTxt := make([]byte, common.HashLength+8)
	copy(hashTxt[:common.HashLength], hash.Bytes())
	binary.LittleEndian.PutUint64(hashTxt[common.HashLength:], tx.Seed())
	digest := crypto.Keccak256(hashTxt[:])
	return new(big.Int).SetBytes(digest[:]).Cmp(target) <= 0
}
