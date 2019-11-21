package statistics

import (
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/core/types"

	"context"
	"database/sql"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestInsert(t *testing.T) {
	run(t, func(t *testing.T, file string, db ntsdb.RD) {
		record := TxRecord{
			"0x29FD65EcE6C4A8E47b2b5d6C8571Eabf21dAF075",
			"0x312ebd816c7a872d92f90a53f04a956f5d69439798c99fee9d5a2665df0ffc25",
			time.Now().UnixNano(),
			types.TransactionTypeTransfer,
		}
		// insert
		db.Exec(func(conn *sql.Conn) error {
			stmt, err := conn.PrepareContext(context.Background(),
				"INSERT INTO txs(addr,tx_timestamp,tx,tx_type) values(?,?,?,?)")
			require.NoError(t, err)
			res, err := stmt.Exec(record.User, record.TxTimestamp, record.TxHash, record.TxType)
			require.NoError(t, err)

			n, err := res.RowsAffected()
			require.NoError(t, err)
			require.Equal(t, int64(1), n)
			return nil
		})
		rows, err := db.Query("select addr,tx,tx_timestamp,tx_type from txs")
		require.NoError(t, err)
		list, err := ToTxRecord(rows)
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, record, list[0])
	})

}
