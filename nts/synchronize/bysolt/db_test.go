package bysolt

import (
	"io/ioutil"
	"os"
	"testing"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func newDB(t *testing.T) (db *ntsdb.LDBDatabase, close func()) {
	name, err := ioutil.TempDir("", "ntsttest")
	require.NoError(t, err)
	db, err = ntsdb.NewLDBDatabase(name, 16, 16)
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(name)
	}
}

func TestDB(t *testing.T) {
	t.Run("todo", func(t *testing.T) {
		db, close := newDB(t)
		defer close()
		//写入
		var maxNumber = uint64(10)
		for i := uint64(1); i <= maxNumber; i++ {
			writeUPUpdateTODO(db, i)
		}
		//遍历
		pre := uint64(0)
		rangeTodos(db, func(number uint64) bool {
			require.Equal(t, pre+1, number)
			pre = number
			return true
		})
		require.Equal(t, maxNumber, pre)

		t.Run("done", func(t *testing.T) {
			//清理一半
			for i := uint64(1); i <= maxNumber/2; i++ {
				upUpdateDone(db, i)
			}
			//再次遍历应该只有后一半
			pre = maxNumber / 2
			rangeTodos(db, func(number uint64) bool {
				require.Equal(t, pre+1, number)
				pre = number
				return true
			})
		})

	})

	t.Run("pointInfo", func(t *testing.T) {
		db, close := newDB(t)
		defer close()
		var maxNumber = uint64(10)

		for i := uint64(1); i <= maxNumber; i++ {
			p := getUnitPoint(db, i)
			require.Nil(t, p)
		}

		for i := uint64(1); i <= maxNumber; i++ {
			writeUnitPoint(db, Point{
				Number:   i,
				Parent:   CalcHash(common.Uint64ToBytes(i - 1)),
				TreeRoot: CalcHash(common.Uint64ToBytes(i)),
			})
		}

		for i := uint64(1); i <= maxNumber; i++ {
			p := getUnitPoint(db, i)
			require.NotNil(t, p)

			want := Point{
				Number:   i,
				Parent:   CalcHash(common.Uint64ToBytes(i - 1)),
				TreeRoot: CalcHash(common.Uint64ToBytes(i)),
			}
			require.Equal(t, want, *p)
		}
	})
	t.Run("last", func(t *testing.T) {
		db, close := newDB(t)
		defer close()
		number := getLastUnitPointNumber(db)
		require.Zero(t, number)

		caces := []uint64{
			1, 10,
			1<<64 - 1,
			1<<64 - 2,
		}
		for _, c := range caces {
			writeLastUnitPointNumber(db, c)
			require.Equal(t, c, getLastUnitPointNumber(db))
		}
	})
}
