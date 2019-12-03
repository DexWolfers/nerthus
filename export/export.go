package export

import (
	"encoding/binary"
	"fmt"
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/console"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/ntsdb"
	"os"
	"time"
)

const (
	versionNo uint32 = 1
)

// 导出单元数据
func ExportChainData(db ntsdb.Database, since uint64, output string) error {
	now := time.Now()
	_, err := os.Stat(output)
	if err == nil || os.IsExist(err) {
		confirm, err := console.Stdin.PromptConfirm("output file existed,replace?")
		switch {
		case err != nil:
			utils.Fatalf("%v", err)
		case confirm:
			os.Remove(output)
		default:
			fmt.Println("cancel export")
			return nil
		}
	}
	count, err := exportFile(db, since, output)
	if err != nil {
		return err
	}
	fmt.Println("export done", "count=", count, "cost=", time.Now().Sub(now))
	return nil
}

func exportFile(db ntsdb.Database, since uint64, output string) (int, error) {

	f, err := os.Create(output)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// 插入文件头数据 4位版本号+4位单元总数
	version := make([]byte, 4)
	binary.BigEndian.PutUint32(version, versionNo)
	units := make([]byte, 4) //预留4位
	if _, err := f.Write(append(version, units...)); err != nil {
		return 0, err
	}

	var (
		count    int
		size     int
		lastTime uint64
	)
	fmt.Println("writing chain raw data to file ", "since=", since, "filename=", f.Name())
	var optErr error
	rawdb.RangUnitByTimline(db, since, func(hash common.Hash) bool {
		header, err := rawdb.GetHeader(db, hash)
		if err != nil {
			utils.Fatal(err)
			optErr = err
		}
		if header.Number == 0 { //忽略创世单元
			return true
		}
		//格式：   长度 + 数据
		b := rawdb.GetRawUnit(db, header.MC, hash, header.Number)

		lenData := make([]byte, 4)
		binary.BigEndian.PutUint32(lenData, uint32(len(b)))
		if _, optErr = f.Write(lenData); optErr != nil {
			return false
		}

		if _, optErr = f.Write(b); optErr != nil {
			return false
		}

		count++
		size += len(b)
		lastTime = header.Timestamp
		fmt.Printf("\r>> exported %d,LastTime %d, Size %s", count, lastTime, common.StorageSize(size))
		return true
	})

	// 写入单元数量到文件头
	binary.BigEndian.PutUint32(units, uint32(count))
	_, optErr = f.WriteAt(units, int64(len(version)))
	fmt.Println("\nwrite data done.", "version=", versionNo, "units=", count, "deadline=", lastTime, "size=", common.StorageSize(size))
	return count, optErr
}
