package rawdb

import (
	"bytes"
	"io"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/syndtr/goleveldb/leveldb/util"
)

func GetTxRLP(db ntsdb.Getter, txhash common.Hash) rlp.RawValue {
	return Get(db, transactionKey(txhash))
}

func WriteTransaction(db ntsdb.Putter, tx *types.Transaction) {
	b, err := rlp.EncodeToBytes(tx)
	if err != nil {
		gameover(err, "failed to encode *types.Transaction")
	}
	err = db.Put(transactionKey(tx.Hash()), b)
	if err != nil {
		gameover(err, "failed to storage Transaction")
	}
}

// 遍历交易
func RangeTxs(db ntsdb.Database, f func(tx []byte) bool) {
	iter := mustLDB(db).NewIterator(util.BytesPrefix(transactionPrefix), nil)
	defer iter.Release()
	for iter.Next() {
		if !f(iter.Value()) {
			break
		}
	}
}

func DecodeTx(b []byte) (*types.Transaction, error) {
	tx := new(types.Transaction)
	err := rlp.DecodeBytes(b, tx)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
func GetTx(db ntsdb.Getter, txHash common.Hash) *types.Transaction {
	b := GetTxRLP(db, txHash)
	if len(b) == 0 {
		return nil
	}
	tx, err := DecodeTx(b)
	if err != nil {
		gameover(err, "failed to decode tx")
	}
	return tx
}

func GetTxListRLP(db ntsdb.Getter, txs []common.Hash) rlp.RawValue {
	buf := bytes.NewBuffer(nil)
	for _, h := range txs {
		buf.Write(GetTxRLP(db, h))
	}
	return buf.Bytes()
}

// 解密交易的 RLP 数据集
func DecodeTxsRLP(b []byte) (types.Transactions, error) {
	return decodeTxs(rlp.NewStream(bytes.NewReader(b), 0))
}
func decodeTxs(stream *rlp.Stream) (types.Transactions, error) {
	var txs types.Transactions
	for {
		tx := new(types.Transaction)
		if err := stream.Decode(tx); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		txs = append(txs, tx)
	}
	return txs, nil
}
