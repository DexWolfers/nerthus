package statistics

import (
	"database/sql"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

type TxRecord struct {
	User        string //hex string
	TxHash      string // hex string
	TxTimestamp int64
	TxType      types.TransactionType
}

type WorkAt struct {
	Chain  common.Address
	Height uint64
}

func ToTxRecord(rows *sql.Rows) (list []TxRecord, err error) {
	for rows.Next() {
		var r TxRecord
		err = rows.Scan(&r.User, &r.TxHash, &r.TxTimestamp, &r.TxType)
		if err != nil {
			return
		}
		list = append(list, r)
	}
	return
}

func toWorkAt(rows *sql.Rows) (list []WorkAt, err error) {
	var chain string
	var height uint64
	for rows.Next() {
		err = rows.Scan(&chain, &height)
		if err != nil {
			return
		}
		list = append(list, WorkAt{
			Chain:  common.StringToAddress(chain),
			Height: height,
		})
	}
	return
}

func toWorkAtMap(rows *sql.Rows) (map[common.Address]uint64, error) {
	var chain string
	var height uint64
	result := make(map[common.Address]uint64)
	for rows.Next() {
		err := rows.Scan(&chain, &height)
		if err != nil {
			return nil, err
		}
		result[common.ForceDecodeAddress(chain)] = height
	}
	return result, nil
}
