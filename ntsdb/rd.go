package ntsdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"

	_ "github.com/mattn/go-sqlite3"
)

type writeReq struct {
	call func(conn *sql.Conn) error
	ret  chan error
}

type RD interface {
	Close() error
	Exec(f func(conn *sql.Conn) error) error
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

type SqliteDB struct {
	*sql.DB
	writes    chan writeReq
	writeConn *sql.Conn

	quit     chan struct{}
	quitDone chan struct{}
}

// NewRD 创建关系数据库，默认使用sqlite，使用sqlite时将使用wal模式，已便支持读写并发
func NewRD(file string) (RD, error) {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?cache=shared&_synchronous=2&_journal_mode=WAL", file))
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, err
	}
	rd := &SqliteDB{DB: db,
		writeConn: conn,
		quit:      make(chan struct{}),
		quitDone:  make(chan struct{}),
		writes:    make(chan writeReq), //并发数为1
	}
	go rd.loop()
	return rd, nil
}

// NewTempRD 创建临时关系库
func NewTempRD() (RD, error) {
	file, err := ioutil.TempFile("nerthus", "sqlite3")
	if err != nil {
		return nil, err
	}
	file.Close()
	return NewRD(file.Name())
}

func (rd *SqliteDB) Close() error {
	close(rd.quit)
	<-rd.quitDone
	rd.writeConn.Close()
	return rd.DB.Close()
}

func (rd *SqliteDB) loop() {
	for {
		select {
		case <-rd.quit:
			//退出前需等待所有未执行完成的写操作执行完成
			for {
				if len(rd.writes) == 0 {
					break
				}
				req := <-rd.writes
				req.ret <- req.call(rd.writeConn)
			}
			rd.writeConn.Close()
			close(rd.quitDone)
			return
		case req := <-rd.writes:
			req.ret <- req.call(rd.writeConn)
		}
	}
}

// Exec 执行SQL，因为SQLite不支持写并发，因此所有写操作必须排队处理。
// 该执行将等待执行，直到处理完成。
func (rd *SqliteDB) Exec(f func(conn *sql.Conn) error) error {
	select {
	case <-rd.quit:
		return errors.New("sqlite service is closed")
	default:
		ret := make(chan error)
		rd.writes <- writeReq{
			call: f,
			ret:  ret,
		}
		return <-ret
	}
}

func (rd *SqliteDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return rd.writeConn.QueryContext(context.Background(), query, args...)
}
func (rd *SqliteDB) QueryRow(query string, args ...interface{}) *sql.Row {
	return rd.writeConn.QueryRowContext(context.Background(), query, args...)
}
