package ntsdb

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func newBadgerdb(t *testing.T) (db *BadgerDB, close func()) {
	dir, err := ioutil.TempDir("", "db")
	require.NoError(t, err)

	db, err = NewBadger(dir)
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(dir)
	}
}

func TestBadgerDB(t *testing.T) {
	s := NewSuite(t, "cachedb", func(t *testing.T) (Database, func()) {
		return newBadgerdb(t)
	})
	s.Run()
}
