package rawdb

import (
	"fmt"
	"math/big"
	"runtime"
	"strings"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	metrics "github.com/rcrowley/go-metrics"
)

var (
	headUnitKey             = []byte("LastUnit")
	headerPrefix            = []byte("h")
	numSuffix               = []byte("n")
	unitHashPrefix          = []byte("H")
	bodyPrefix              = []byte("b")
	uintDataPrefix          = []byte("ud_")
	unitReceiptsPrefix      = []byte("r")
	voteMsgPrefix           = []byte("vote_")
	unitChildrenPrefix      = []byte("uc_") //记录单元的子单元 , key= "uc_" + unit Hash  + Index ,value= children Unit hash
	unitChildrenCountSuffix = []byte("_c")  // 记录单元的子单元数量： key= "uc_" + unit Hash + "_c" ,value= count
	transactionPrefix       = []byte("utx_")
	transactionStatusPrefix = []byte("tx_status_")
	chainsPrefix            = []byte("chains")
	preimagePrefix          = []byte("secure-key-") // preimagePrefix + hash -> preimage
	mipmapPre               = []byte("mipmap-log-bloom-")
	MIPMapLevels            = []uint64{1000000, 500000, 100000, 50000, 1000}
	configPrefix            = []byte("nerthus-config-") // config prefix for the db
	badUnitPrefix           = []byte("bu_")             //bad unit
	witnessPowTxPrefix      = []byte("witness-pow-tx")
	arbitrationResultPrefix = []byte("arb_r_")  //仲裁结果（在单元稳定后进行登记的内容）
	arbitrationInPrefix     = []byte("arb_in_") //记录该链在登记中（在单元稳定后进行登记的内容）
	prefix_clean            = []byte("cln_")    // statedb清理截止高度前缀
	preimageCounter         = metrics.NewRegisteredCounter("db/preimage/total", nil)
	preimageHitCounter      = metrics.NewRegisteredCounter("db/preimage/hits", nil)
)

//
// Prefix 获取body db key
func unitBodyPrefix(uHash common.Hash) []byte {
	return makeKey(bodyPrefix, uHash)
}

// getBodySignPrefix 获取body签名的db key
func unitSignKey(uHash common.Hash) []byte {
	return makeKey(unitBodyPrefix(uHash), []byte("sign"))
}
func unitDataPrefix(uhash common.Hash, number uint64) []byte {
	return makeKey(uintDataPrefix, uhash, number)
}

func unitBodyKey(chain common.Address, hash common.Hash, number uint64) []byte {
	return makeKey(chain, "_u_", number, hash, "_b")
}

func unitOriginTxsKey(uhash common.Hash, number uint64) []byte {
	return makeKey(unitDataPrefix(uhash, number), []byte("_txhashs"))
}

func unitTxStepExecKey(uhash common.Hash, number uint64) []byte {
	return makeKey(unitDataPrefix(uhash, number), []byte("_txs"))
}

func headerKey(uhash common.Hash) []byte {
	return makeKey(headerPrefix, uhash)
}
func unitVoteSignsKey(uhash common.Hash, number uint64) []byte {
	return makeKey(voteMsgPrefix, uhash)
}

// unitReceiptPrefix = unitReceiptsPrefix + num (uint64 big endian) + hash
func unitReceiptPrefix(number uint64, hash common.Hash) []byte {
	return append(append(unitReceiptsPrefix, common.Uint64ToBytes(number)...), hash.Bytes()...)
}

func transactionKey(txHash common.Hash) []byte {
	return makeKey(transactionPrefix, txHash)
}
func CleanerKey(chain common.Address) []byte {
	return append(prefix_clean, chain.Bytes()...)
}

func Get(db ntsdb.Getter, key []byte) []byte {
	b, err := db.Get(key)
	if err != nil {
		// TODO: 这是一种坏味道的判断方式，需要在ntsdb中为所有数据库取数错误定义一个 ErrNotFound
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		gameover(err, "failed to get data by key", 2)
	}
	return b
}

// makeKey 拼接data
func makeKey(data ...interface{}) []byte {
	length := len(data)
	var out []byte
	for i := 0; i < length; i++ {
		switch val := data[i].(type) {
		case int:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case int64:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case uint64:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case string:
			out = append(out, []byte(val)...)
		case []byte:
			out = append(out, val...)
		case *big.Int:
			out = append(out, val.Bytes()...)
		case common.Address:
			out = append(out, val.Bytes()...)
		case common.Hash:
			out = append(out, val.Bytes()...)
		default:
			gameover(errors.Errorf("can not parse %T to byte", data[i]), "failed to parse")
		}
	}
	return out
}

func gameover(err error, msg string, skip ...int) {
	s := 1
	if len(skip) > 0 {
		s = skip[0]
	}
	pc, file, line, _ := runtime.Caller(s)
	f := runtime.FuncForPC(pc)
	fmt.Printf(`%s at %s
file: %s:%d
 err: %s`, msg, f.Name(), file, line, err)
	log.Crit(fmt.Sprintf("rawdb:%s", msg), "file", file, "func", f.Name(), "line", line, "err", err)
}
