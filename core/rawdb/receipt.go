package rawdb

import (
	"encoding/binary"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
)

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
	keyPrefix := unitReceiptPrefix(number, hash)

	table := ntsdb.NewTable(db, keyPrefix)

	// Convert the receipts into their storage form and serialize them
	for i, receipt := range receipts {
		data, err := rlp.EncodeToBytes((*types.ReceiptForStorage)(receipt))
		if err != nil {
			log.Crit("rlp failed", "err", err)

		}
		if err := table.Put(uint16ToBytes(uint16(i)), data); err != nil {
			log.Crit("storage receipt failed", "uhash", hash, "number", number, "index", i, "err", err)
		}
	}

	//成功后再写入
	if err := table.Put([]byte("_count"), uint16ToBytes(uint16(len(receipts)))); err != nil {
		log.Crit("storage receipt count failed",
			"uhash", hash, "number", number, "count", len(receipts), "err", err)

	}
}

// 获取单元指定位置的交易回执
func GetUnitReceiptAt(db ntsdb.Database, uhash common.Hash, number uint64, index uint16) *types.Receipt {
	table := ntsdb.NewTable(db, unitReceiptPrefix(number, uhash))
	data, _ := table.Get(uint16ToBytes(index))
	if len(data) == 0 {
		return nil
	}
	var storageReceipt types.ReceiptForStorage
	if err := rlp.DecodeBytes(data, &storageReceipt); err != nil {
		log.Crit("invalid receipt array RLP", "err", err)
	}
	return (*types.Receipt)(&storageReceipt)
}

// GetUnitReceipt 获取指定单元的交易回执
func GetUnitReceipts(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) types.Receipts {
	table := ntsdb.NewTable(db, unitReceiptPrefix(number, hash))

	b, _ := table.Get([]byte("_count"))
	if len(b) == 0 {
		return nil
	}
	count := bytesToUint16(b)
	list := make(types.Receipts, count)

	for i := uint16(0); i < count; i++ {
		data, _ := table.Get(uint16ToBytes(i))
		if len(data) == 0 {
			log.Warn("missing receipt", "uhash", hash, "number", number, "index", i)
			return nil
		}
		var storageReceipt types.ReceiptForStorage
		if err := rlp.DecodeBytes(data, &storageReceipt); err != nil {
			log.Crit("invalid receipt array RLP", "hash", hash, "err", err)
		}
		list[i] = (*types.Receipt)(&storageReceipt)
	}
	return list
}

// DeleteUnitReceipt 删除单元receipt信息
func DeleteUnitReceipt(db ntsdb.Database, mcaddr common.Address, hash common.Hash, number uint64) {
	table := ntsdb.NewTable(db, unitReceiptPrefix(number, hash))
	countKey := []byte("_count")
	b, _ := table.Get(countKey)
	if len(b) == 0 {
		return
	}
	count := bytesToUint16(b)

	for i := uint16(0); i < count; i++ {
		if err := table.Delete(uint16ToBytes(uint16(i))); err != nil {
			log.Crit("delete receipt failed", "uhash", hash, "number", number, "index", i)
		}
	}
	if err := table.Delete(countKey); err != nil {
		log.Crit("delete receipt count failed", "uhash", hash, "number", number, "count", count)
	}
}
