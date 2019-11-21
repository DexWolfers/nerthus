package statistics

import (
	"context"
	"database/sql"

	"gitee.com/nerthus/nerthus/ntsdb"
)

func checkTable(db ntsdb.RD) error {
	//check table
	return db.Exec(func(conn *sql.Conn) error {
		_, err := conn.ExecContext(context.Background(), `
CREATE TABLE  IF NOT EXISTS  "txs" ( "addr" TEXT, "tx" TEXT, "tx_timestamp" INTEGER, "tx_type" INTEGER);
CREATE TABLE  IF NOT EXISTS "txs_tmp" ( "addr" TEXT, "tx" TEXT, "tx_timestamp" INTEGER, "tx_type" INTEGER);
CREATE TABLE  IF NOT EXISTS "wkat" ("chain" TEXT,"height" INTEGER, PRIMARY KEY("chain"));
CREATE TABLE  IF NOT EXISTS "wkat_tmp" ("chain" TEXT,"height" INTEGER);
CREATE UNIQUE INDEX  IF NOT EXISTS "txs_addr_tx" ON "txs" ( "addr" ASC, "tx" ASC );
CREATE INDEX IF NOT EXISTS  "txs_index_query" ON "txs" ( "addr", "tx_type" ASC, "tx_timestamp" DESC, "tx" );
CREATE INDEX IF NOT EXISTS  "txs_index_query2" ON "txs" ( "addr", "tx_timestamp" DESC, "tx" );
`)
		return err
	})
}

func toHistory(db ntsdb.RD) error {
	//check table
	return db.Exec(func(conn *sql.Conn) error {
		// to history
		//每次启动时，将三个月前的交易迁移到历史库中
		sql := `CREATE TABLE IF NOT EXISTS "txs_his" ( "addr" TEXT, "tx" TEXT, "tx_timestamp" INTEGER, "tx_type" INTEGER);
INSERT INTO txs_his SELECT addr,tx,tx_timestamp,tx_type FROM txs where tx_timestamp < ( strftime('%s','now') - 60*60*24*90);
`
		_, err := conn.ExecContext(context.Background(), sql)
		return err
	})
}
