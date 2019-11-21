// Author: @kulics

package core

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/metrics"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrUnitNotFind         = errors.New("unit not find")
	ErrLastUnitNotFind     = errors.New("last unit not find")
	ErrMissingTransaction  = errors.New("missing transaction information")
	ErrMissingTxStatusInfo = errors.New("txStatusInfo not find")
	ErrInvalidScTxSender   = errors.New("invalid sc contract sender")
)

var (
	headUnitKey             = []byte("LastUnit")
	headerPrefix            = []byte("h")
	numSuffix               = []byte("n")
	unitHashPrefix          = []byte("H")
	bodyPrefix              = []byte("b")
	uintDataPrefix          = []byte("ud_")
	unitReceiptsPrefix      = []byte("r")
	unitChildrenPrefix      = []byte("uc_") //记录单元的子单元 , key= "uc_" + unit Hash  + Index ,value= children Unit hash
	unitChildrenCountSuffix = []byte("_c")  // 记录单元的子单元数量： key= "uc_" + unit Hash + "_c" ,value= count
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
	preimageCounter         = metrics.NewRegisteredCounter("db/preimage/total", nil)
	preimageHitCounter      = metrics.NewRegisteredCounter("db/preimage/hits", nil)
)

type DBPuter interface {
	Put(key, value []byte) error
}

// WriteStableHash 存储单元位置与单元Hash关系
func WriteStableHash(db DBPuter, hash common.Hash, mcaddr common.Address, number uint64) {
	key := append(append(append(headerPrefix, mcaddr.Bytes()...), common.Uint64ToBytes(number)...), numSuffix...)
	if err := db.Put(key, hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// GetStableHash 获取账户链上指定编号的合法单元hash.
func GetStableHash(db ntsdb.Database, chain common.Address, number uint64) common.Hash {
	if number == 0 { //获取时0高度信息则均指向创世
		chain = params.SCAccount
	}
	key := append(append(append(headerPrefix, chain.Bytes()...), common.Uint64ToBytes(number)...), numSuffix...)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

func DeleteStableHash(db ntsdb.Database, hash common.Hash, chain common.Address, number uint64) error {
	key := append(append(append(headerPrefix, chain.Bytes()...), common.Uint64ToBytes(number)...), numSuffix...)
	return db.Delete(key)
}

//MissingNumber 当单元头未存在于DB时返回一个缺失的编号
const MissingNumber = uint64(0xffffffffffffffff)

type unitLocationEntry struct {
	MC     common.Address
	Number uint64
}

// GetChainTailHash 获取指定链的当前稳定单元哈希
func GetChainTailHash(db ntsdb.Database, mcaddr common.Address) common.Hash {
	key := append(headUnitKey, mcaddr.Bytes()...)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// GetChainTailHead 获取指定链的当前稳定单元哈希
func GetChainTailHead(db ntsdb.Database, mcaddr common.Address) *types.Header {
	hash := GetChainTailHash(db, mcaddr)
	if hash.Empty() {
		return nil
	}
	return GetHeader(db, hash)
}

// WriteChainTailHash 存储单元头Hash到账户链末尾
func WriteChainTailHash(db DBPuter, hash common.Hash, mcaddr common.Address) error {
	key := append(headUnitKey, mcaddr.Bytes()...)
	if err := db.Put(key, hash.Bytes()); err != nil {
		log.Crit("Failed to store last unit's hash", "err", err)
		return err
	}
	return nil
}

// DeleteHeader 删除单元头信息
func DeleteHeader(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) {
	h := GetHeader(db, hash)
	if h != nil {
		rawdb.DelTimeline(db, h.Timestamp, hash)
	}

	db.Delete(append(unitHashPrefix, hash.Bytes()...))
	db.Delete(append(headerPrefix, hash.Bytes()...))
}

// 判断是否存在单元
func ExistUnitNumber(db ntsdb.Getter, hash common.Hash) bool {
	data, _ := db.Get(append(unitHashPrefix, hash.Bytes()...))
	if len(data) == 0 {
		return false
	}
	return true
}

// GetUnitNumber 通过单元哈希获取获取指定单元信息（账户链，编号）
func GetUnitNumber(db ntsdb.Getter, hash common.Hash) (common.Address, uint64) {
	data, _ := db.Get(append(unitHashPrefix, hash.Bytes()...))
	if len(data) == 0 {
		return common.EmptyAddress, MissingNumber
	}
	loca := new(unitLocationEntry)
	if err := rlp.Decode(bytes.NewReader(data), loca); err != nil {
		log.Error("Invalid unit location RLP", "hash", hash, "err", err)
		return common.EmptyAddress, MissingNumber
	}
	return loca.MC, loca.Number
}

// GetHeader 获取单元头，如果未找到返回空。
func GetHeader(db ntsdb.Database, hash common.Hash) *types.Header {
	h, err := rawdb.GetHeader(db, hash)
	if err != nil { //实际上不需要关注 Error
		return nil
	}
	return h
}

// GetHeaderByHash 直接通过单元hash获取单元头，如果未找到返回空。
func GetHeaderByHash(db ntsdb.Database, hash common.Hash) *types.Header {
	return GetHeader(db, hash)
}

// DeleteBody 删除body落地数据
func DeleteBody(db ntsdb.Database, chain common.Address, hash common.Hash, number uint64) {
	rawdb.DelUnitBody(db, chain, number, hash)
}

// WriteBody 序列化存储单元Body到db。
func WriteBody(db ntsdb.PutGetter, chain common.Address, number uint64, uHash common.Hash, body *types.Body, receipts types.Receipts) error {
	// 写入body.txs
	txCount := uint64(body.Txs.Len())
	var failed bool
	txCountList := make(map[common.Hash]uint64, txCount)
	var index uint64

	for k := range body.Txs {
		index = uint64(k + 1)

		failed = false
		if len(receipts)-1 >= k {
			// 注意，这里只是方便一些测试代码无receipts情况。
			// 正常单元的 len(receipts) 必须等于 len(txs)
			failed = receipts[k].Failed
		}
		tx := body.Txs[k]

		count, err := addTransactionStatus(db, tx.TxHash, TxStatusInfo{
			Action:      tx.Action,
			UnitHash:    uHash,
			Failed:      failed,
			TxExecIndex: index,
		}, txCountList[body.Txs[k].TxHash])
		if err != nil {
			return err
		}

		txCountList[tx.TxHash] = count
	}

	rawdb.WriteUnitBody(db, chain, uHash, number, body)

	return nil
}

// 处理特殊交易日志
func handleSpecialLogs(db ntsdb.PutGetter, unit *types.Unit, logs []*types.Log) {
	if unit.MC() != params.SCAccount || len(logs) == 0 {
		return
	}
	//检查新链出现
	newChains := make([]common.Address, 0, len(logs))

	for _, l := range logs {
		if l.ContainsTopic(sc.GetEventTopicHash(sc.ChainStatusChangedEvent)) {
			info, err := sc.UnpackChainStatusChangedEvent(l.Data)
			if err != nil {
				log.Crit("failed to call UnpackChainStatusChangedEvent", "err", err)
			}
			if info.First && info.Status == sc.ChainStatusNormal {
				newChains = append(newChains, info.Chain)
			}
		}
	}
}

// GetTransaction 从db中获取指定交易
func GetTransaction(db ntsdb.Getter, txHash common.Hash) *types.Transaction {
	return rawdb.GetTx(db, txHash)
}

// ExistBody 根据单元hash检查body是否存在
func ExistBody(db ntsdb.Getter, hash common.Hash) bool {
	mc, number := GetUnitNumber(db, hash)
	if mc.Empty() {
		return false
	}
	return rawdb.ExistUnitBody(db, mc, hash, number)
}

// GetBody 通过单元hash获得单元内容（交易+签名.
// 如果未找到则返回空.
func GetBody(db ntsdb.Database, chain common.Address, number uint64, uHash common.Hash) *types.Body {
	return rawdb.GetUnitBody(db, chain, uHash, number)
}

// WriteUnit serializes a unit into the database, header and body separately.
func WriteUnit(db ntsdb.PutGetter, unit *types.Unit, receipts types.Receipts) error {
	// Store the body first to retain database consistency
	if err := WriteBody(db, unit.MC(), unit.Number(), unit.Hash(), unit.Body(), receipts); err != nil {
		return err
	}
	rawdb.WriteHeader(db, unit.Header())
	// 存储引用关系
	if err := WriteChildrens(db, unit); err != nil {
		return err
	}
	return nil
}

// GetUnit 通过单元获取完整单元数据，包括Payload，Payload为空则返回空
func GetUnit(db ntsdb.Database, hash common.Hash) *types.Unit {
	return rawdb.GetUnit(db, hash)
}

// returns a formatted MIP mapped key by adding prefix, canonical number and level
//
// ex. fn(98, 1000) = (prefix || 1000 || 0)
func mipmapKey(num, level uint64) []byte {
	lkey := make([]byte, 8)
	binary.BigEndian.PutUint64(lkey, level)
	key := new(big.Int).SetUint64(num / level * level)

	return append(mipmapPre, append(lkey, key.Bytes()...)...)
}

// GetChainConfig 通过给定的hash获取chain配置
func GetChainConfig(db ntsdb.Database, hash common.Hash) (*params.ChainConfig, error) {
	jsonChainConfig, _ := db.Get(append(configPrefix, hash.Bytes()...))
	if len(jsonChainConfig) == 0 {
		return nil, ErrChainConfigNotFound
	}

	var config params.ChainConfig
	err := json.Unmarshal(jsonChainConfig, &config)
	return &config, err
}

// WriteChainConfig 存储chain 配置到db
func WriteChainConfig(db ntsdb.Putter, hash common.Hash, cfg *params.ChainConfig) error {
	// 如果为空则不存储，Get时将拉取默认配置
	if cfg == nil {
		return nil
	}

	jsonChainConfig, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return db.Put(append(configPrefix, hash[:]...), jsonChainConfig)
}

func uint16ToBytes(n uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, n)
	return buf
}
func bytesToUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

// WriteUnitReceipt  将单元下所有交易凭据作为一个凭据组合。 T
func WriteUnitReceipts(db ntsdb.Database, hash common.Hash, mcaddr common.Address, number uint64, receipts types.Receipts) {
	rawdb.WriteUnitReceipts(db, hash, mcaddr, number, receipts)
}

func GetTxReceipts(db ntsdb.Database, txHash common.Hash) (types.Receipts, error) {
	var txReceipts types.Receipts
	err := GetTransactionStatus(db, txHash, func(info TxStatusInfo) (b bool, e error) {
		chain, number := GetUnitNumber(db, info.UnitHash)
		if chain.Empty() {
			return false, errors.New("not found unit info by hash")
		}
		receipts := GetUnitReceiptAt(db, info.UnitHash, number, uint16(info.TxExecIndex-1))
		if receipts == nil {
			return false, fmt.Errorf("not found tx receipt at unit:chain=%s,uhash=%s,number=%d",
				chain, info.UnitHash.String(), number)
		}
		txReceipts = append(txReceipts, receipts)
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return txReceipts, nil
}

// 获取单元指定位置的交易回执
func GetUnitReceiptAt(db ntsdb.Database, uhash common.Hash, number uint64, index uint16) *types.Receipt {
	return rawdb.GetUnitReceiptAt(db, uhash, number, index)
}

// GetUnitReceipt 获取指定单元的交易回执
func GetUnitReceipts(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) types.Receipts {
	return rawdb.GetUnitReceipts(db, mcaddr, hash, number)
}

// DeleteUnitReceipt 删除单元receipt信息
func DeleteUnitReceipt(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) {
	rawdb.DeleteUnitReceipt(db, mcaddr, hash, number)
}

//// txLookupEntry is a positional metadata to help looking up the data content of
//// a transaction or receipt given only its hash.
//type txLookupEntry struct {
//	UnitHash   common.Hash
//	UnitMC     common.Address
//	UnitNumber uint64
//	SCNumber   uint64
//	SCHash     common.Hash
//	Index      uint64
//}

//func DeleteTxLookupEntry(db ntsdb.Database, hash common.Hash) {
//	db.Delete(append(lookupPrefix, hash.Bytes()...))
//}

// 记录一笔交易的不同阶段信息
type TxStatusInfo struct {
	UnitHash    common.Hash    `json:"unit_hash"`
	Action      types.TxAction `json:"action"`
	Failed      bool           `json:"failed"`
	TxExecIndex uint64         //从1开始的索引
}

// GetTransactionStatus 迭代交易类型
func GetTransactionStatus(db ntsdb.PutGetter, txHash common.Hash, fun func(info TxStatusInfo) (bool, error)) error {
	count := getTransactionStatusCount(db, txHash)
	if count == 0 {
		return ErrMissingTxStatusInfo
	}
	for i := uint64(1); i <= count; i++ {
		info, err := getTransactionStatusByFlag(db, txHash, i)
		if err != nil {
			return err
		}
		if ok, err := fun(info); err != nil {
			return err
		} else if !ok {
			return nil
		}
	}
	return nil
}

// GetLastTransactionStatus 获取交易的最后一个类型
func GetLastTransactionStatus(db ntsdb.PutGetter, txHash common.Hash) (TxStatusInfo, error) {
	count := getTransactionStatusCount(db, txHash)
	if count == 0 {
		return TxStatusInfo{}, ErrMissingTransaction
	}
	return getTransactionStatusByFlag(db, txHash, count)
}

// delTransactionStatus 删除交易类型
func delTransactionStatus(db ntsdb.Database, uhash common.Hash, txHash common.Hash, action types.TxAction) error {
	var infoList []TxStatusInfo
	GetTransactionStatus(db, txHash, func(info TxStatusInfo) (b bool, e error) {
		if !(info.Action == action && info.UnitHash == uhash) {
			infoList = append(infoList, info)
		}
		return true, nil
	})
	// 初始化列表数
	countKey := getTransactionStatusCountPrefix(txHash)
	if err := db.Put(countKey, common.Uint64ToBytes(0)); err != nil {
		return err
	}
	var count uint64 = 0
	// 写入不能跟原先的顺序不一致
	for i := len(infoList); i > 0; i-- {
		_, err := addTransactionStatus(db, txHash, infoList[i-1], count)
		if err != nil {
			return err
		}
	}
	return nil
}

// getTransactionStatusInfoPrefix 获取交易类型详情db key
func getTransactionStatusInfoPrefix(txHash common.Hash, flag uint64) []byte {
	return append(getTransactionStatusPrefix(txHash), common.Uint64ToBytes(flag)...)
}

// getTransactionStatusByFlag 根据索引获取交易类型详情
func getTransactionStatusByFlag(db ntsdb.PutGetter, txHash common.Hash, flag uint64) (TxStatusInfo, error) {
	var info TxStatusInfo
	key := getTransactionStatusInfoPrefix(txHash, flag)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return info, errors.New("transactionStatus not find")
	}
	err := rlp.DecodeBytes(data, &info)
	return info, err
}

// getTransactionStatusPrefix 获取交易类型db key
func getTransactionStatusPrefix(txHash common.Hash) []byte {
	return append(transactionStatusPrefix, txHash.Bytes()...)
}

// addTransactionStatus 添加交易类型
func addTransactionStatus(db ntsdb.PutGetter, txHash common.Hash, info TxStatusInfo, count uint64) (uint64, error) {
	if count == 0 {
		count = getTransactionStatusCount(db, txHash)
	}
	count += 1
	return count, writeTransactionStatus(db, txHash, info, count)
}

// writeTransactionStatus 写入交易类型
func writeTransactionStatus(db ntsdb.Putter, txHash common.Hash, info TxStatusInfo, flag uint64) error {
	buf, err := rlp.EncodeToBytes(info)
	if err != nil {
		return err
	}
	if err = writeTransactionStatusCount(db, txHash, flag); err != nil {
		return err
	}
	key := getTransactionStatusInfoPrefix(txHash, flag)
	return db.Put(key, buf)
}

// getTransactionStatusCountPrefix 获取交易类型总数db key
func getTransactionStatusCountPrefix(txHash common.Hash) []byte {
	return append(getTransactionStatusPrefix(txHash), 'c')
}

// getTransactionStatusCount 获取交易类型总数
func getTransactionStatusCount(db ntsdb.PutGetter, txHash common.Hash) uint64 {
	key := getTransactionStatusCountPrefix(txHash)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return 0
	}
	return common.BytesToUInt64(data)
}

// writeTransactionStatusCount 写入交易类型总数
func writeTransactionStatusCount(db ntsdb.Putter, txHash common.Hash, count uint64) error {
	key := getTransactionStatusCountPrefix(txHash)
	return db.Put(key, common.Uint64ToBytes(count))
}

// WriteMipmapBloom writes each address included in the receipts' logs to the
// MIP bloom bin.
func WriteMipmapBloom(db ntsdb.Database, mcaddr common.Address, number uint64, receipts types.Receipts) error {

	// 不需要锁，外部已锁

	for _, level := range MIPMapLevels {
		key := mipmapKey(number, level)
		key = append(mcaddr.Bytes(), key...)

		bloomDat, _ := db.Get(key)
		bloom := types.BytesToBloom(bloomDat)
		for _, r := range receipts {
			for _, log := range r.Logs {
				bloom.Add(log.Address.Big())
			}
		}
		db.Put(key, bloom.Bytes())
	}
	return nil
}

// PreimageTable returns a Database instance with the key prefix for preimage entries.
func PreimageTable(db ntsdb.Database) ntsdb.Database {
	return ntsdb.NewTable(db, preimagePrefix)
}

// WritePreimages writes the provided set of preimages to the database. `number` is the
// current unit number, and is used for debug messages only.
func WritePreimages(db ntsdb.Database, mcaddr common.Address, number uint64, preimages map[common.Hash][]byte) error {
	table := PreimageTable(db)
	hitCount := 0
	for hash, preimage := range preimages {
		if _, err := table.Get(hash.Bytes()); err != nil {
			db.Put(hash.Bytes(), preimage)
			hitCount++
		}
	}
	preimageCounter.Inc(int64(len(preimages)))
	preimageHitCounter.Inc(int64(hitCount))
	return nil
}

// DeleteUnit 根据hash移除此区块全部信息
func DeleteUnit(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) {
	DeleteHeader(db, mcaddr, hash, number)
	DeleteBody(db, mcaddr, hash, number)
	DeleteUnitReceipt(db, mcaddr, hash, number)
	DeleteChildrenRelation(db, hash)

	//清理投票信息
	db.Delete(makeKey(voteMsgPrefix, hash.Bytes()))
}

// DeleteChildrenRelation 删除关系
//	注意，此处还无法直接将此单元信息从他的父单元关系下移除
func DeleteChildrenRelation(db ntsdb.Database, hash common.Hash) {
	countKey := append(append(unitChildrenPrefix, hash.Bytes()...), unitChildrenCountSuffix...)

	data, _ := db.Get(countKey)
	if len(data) == 0 {
		return
	}
	count := new(big.Int).SetBytes(data)

	prefix := append(unitChildrenPrefix, hash.Bytes()...)
	big1 := big.NewInt(1)
	//不断获取
	for count.Sign() > 0 {
		db.Delete(append(prefix, count.Bytes()...))
		count.Sub(count, big1)
	}
}

var hashChildrenLock sync.Map

// WriteChildrens 记录单元引用关系，将记载单元的子单元数量和子单元。
func WriteChildrens(db ntsdb.PutGetter, unit *types.Unit) error {
	//将单元中记录的引用关系进行保存
	// key= 父单元 ，value= 子单元 ，目的是为了可以一次性获取单元的一级子单元
	//	 但是，因为一个单元的一级子单元也许非常多。
	// 特别是系统链单元，会被成千上万单元引用，故不能直接将单元的所有一级子单元作为一个集合存放。
	// 当前方案：
	//    1. 使用一个 key-value 对 单元的子单元数量进行计数
	//    2. 在使用 N 个 key-value 保存引用关系。
	// 			如： key= unithash_children0  value= children0Hash , key= unithash_children1  value= children1Hash

	// 收集父单元
	parents := make(map[common.Hash]struct{})
	parents[unit.ParentHash()] = struct{}{}
	parents[unit.SCHash()] = struct{}{}
	for _, tx := range unit.Transactions() {
		if tx.PreStep.Hash.Empty() {
			continue
		}
		// 交易中所包含的引用也需要记录
		parents[tx.PreStep.Hash] = struct{}{}
	}
	unitBytes := unit.Hash().Bytes()
	var (
		count     uint64 = 0
		countByte []byte
		//countKey  = make([]byte, 0, 37)
		countKey []byte
	)
	for p := range parents {
		if p.Empty() {
			continue
		}
		//更新计数，需要实时提交，因为外部处理时并发处理。如果同时两个单元同时引用一个父单元，
		// 则对此父单元数据的更新，将出现并发，需要立即提交。
		count = 0
		countKey = append(append(unitChildrenPrefix, p.Bytes()...), unitChildrenCountSuffix...)
		//countKey := makeKey()
		a, _ := hashChildrenLock.LoadOrStore(p, &sync.Mutex{})
		a.(*sync.Mutex).Lock()
		data, _ := db.Get(countKey)
		if len(data) > 0 {
			count = common.BytesToUInt64(data)
		}
		count += 1
		countByte = common.Uint64ToBytes(count)
		db.Put(countKey, countByte)
		a.(*sync.Mutex).Unlock()   //更新后立即释放
		hashChildrenLock.Delete(p) //释放资料
		if err := db.Put(append(append(unitChildrenPrefix, p.Bytes()...), countByte...), unitBytes); err != nil {
			return err
		}
	}
	return nil
}

// WalkChildren 遍历指定父单元的所有子单元。
// cb 方法返回参数用于标记是否退出遍历，如果为false，则继续遍历直到完毕。
// Author: @ysqi
func WalkChildren(db ntsdb.Database, hash common.Hash, cb func(children common.Hash) bool) {
	if cb == nil {
		return
	}
	prefix := append(unitChildrenPrefix, hash.Bytes()...)

	var count uint64 = 0
	countKey := append(prefix, unitChildrenCountSuffix...)
	data, _ := db.Get(countKey)
	if len(data) > 0 {
		count = common.BytesToUInt64(data)
	}

	// 按写入顺序输出
	for i := uint64(1); i <= count; i++ {
		data, _ := db.Get(makeKey(prefix, i))
		if len(data) > 0 {
			stop := cb(common.BytesToHash(data))
			if stop {
				break
			}
		}
	}
}

// Author: @Rg
func WriteWitnessPowTx(db ntsdb.Database, addr common.Address, tx *types.Transaction) error {
	key := append(witnessPowTxPrefix, addr.Bytes()...)
	data, err := rlp.EncodeToBytes(tx)
	if err != nil {
		return err
	}
	if err = db.Put(key, data); err != nil {
		return err
	}
	return nil
}

// Author: @Rg
func ReadWitenessPowTx(db ntsdb.Database, addr common.Address) (*types.Transaction, error) {
	key := append(witnessPowTxPrefix, addr.Bytes()...)
	data, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	var tx types.Transaction
	err = rlp.DecodeBytes(data, &tx)
	return &tx, err
}

// Author: @Rg
func DelWitnessPowTx(db ntsdb.Database, addr common.Address) error {
	key := append(witnessPowTxPrefix, addr.Bytes()...)
	err := db.Delete(key)
	return err
}

func GetVoteMsg(db ntsdb.Database, unitHash common.Hash) ([]types.WitenssVote, error) {
	mc, number := GetUnitNumber(db, unitHash)
	if mc.Empty() {
		return nil, nil
	}
	info := rawdb.GetUnitBodyStorage(db, mc, unitHash, number)
	if info == nil {
		return nil, nil
	}
	return info.Voters, nil
}

func getChainsCountPrefix() []byte {
	return makeKey(chainsPrefix, "count")
}

func readChainCount(db ntsdb.Getter) uint64 {
	data, _ := db.Get(getChainsCountPrefix())
	if len(data) == 0 {
		return 0
	}
	return common.BytesToUInt64(data)
}
