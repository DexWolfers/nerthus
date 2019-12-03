package ntsdb

import (
	"context"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSqliteDB_DefaultConfig(t *testing.T) {
	d, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.Remove(d)

	rd, err := NewRD(filepath.Join(d, "d.db3"))
	require.NoError(t, err)

	configs := map[string]string{
		"journal_mode": "wal",
		"synchronous":  "2",
	}

	for k, v := range configs {
		row := rd.QueryRow(fmt.Sprintf("PRAGMA %s", k))
		require.NoError(t, err)
		var info string
		row.Scan(&info)
		require.Equal(t, v, info)
	}
}

func TestSqliteDB_Exec(t *testing.T) {
	d, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.Remove(d)

	rd, err := NewRD(filepath.Join(d, "d.db3"))
	require.NoError(t, err)

	err = rd.Exec(func(conn *sql.Conn) error {
		_, err := conn.ExecContext(context.Background(), "create table info (c number)")
		return err
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	count := 100
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(i int) {
			defer func() {
				if err := recover(); err != nil {
					t.Fatal(err)
				}
			}()
			defer wg.Done()

			for i := 0; i < 100; i++ {
				if i%5 == 0 {
					err := rd.Exec(func(conn *sql.Conn) error {
						_, err := conn.ExecContext(context.Background(),
							"insert into info (c) values(?)", i)
						return err
					})
					assert.NoError(t, err)
				} else if i%4 == 0 {
					err := rd.Exec(func(conn *sql.Conn) error {
						_, err := conn.ExecContext(context.Background(),
							"update info set c=c*2 where c<?", i)
						return err
					})
					assert.NoError(t, err)
				} else {
					rows, err := rd.Query("select c from info where c>?", i)
					for rows.Next() {
						var c int
						rows.Scan(&c)
					}
					rows.Close()
					assert.NoError(t, err)
				}
			}

		}(i)
	}
	wg.Wait()

}
