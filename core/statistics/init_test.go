package statistics

import (
	"io/ioutil"
	"os"
	"testing"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	run(t, func(t *testing.T, _ string, db ntsdb.RD) {
		err := checkTable(db)
		require.NoError(t, err)
	})
}

func run(t *testing.T, f func(t *testing.T, file string, db ntsdb.RD)) {
	file, err := ioutil.TempFile("", "cntssqlite")
	require.NoError(t, err)
	file.Close()
	defer os.Remove(file.Name())
	db, err := ntsdb.NewRD(file.Name())
	defer db.Close()
	t.Log("db file:", file.Name())

	err = checkTable(db)
	require.NoError(t, err)

	f(t, file.Name(), db)
}
