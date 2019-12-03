package export

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/utils"
	"io"
	"os"
	"time"
)

func ImportChainData(ctx context.Context, nts *core.DagChain, source string) error {
	now := time.Now()
	err := importData(ctx, source, nts)
	fmt.Println("\nimport chain data done", "cost=", time.Now().Sub(now), "err=", err)
	return err
}

func importData(ctx context.Context, source string, dagChain *core.DagChain) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()

	// 读取文件头8位 4位版本号+4位单元总数
	buffer := make([]byte, 4)
	if _, err = f.Read(buffer); err != nil {
		return err
	}
	version := binary.BigEndian.Uint32(buffer)
	if _, err = f.Read(buffer); err != nil {
		return err
	}
	unitCount := binary.BigEndian.Uint32(buffer)

	fmt.Println("prepare import", "version=", version, "units=", unitCount)
	ch := make(chan []byte, 1024*1024)
	wait := make(chan struct{})
	defer func() {
		<-wait
		close(ch)
	}()
	go loopImport(ctx, dagChain, int(unitCount), ch, wait)

	lenData := make([]byte, 4)
	//r := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		//读取长度
		n, err := f.Read(lenData)
		if err == io.EOF {
			fmt.Println("\nImport done")
			return nil
		} else if n != len(lenData) {
			return errors.New("invalid data")
		}
		size := binary.BigEndian.Uint32(lenData)

		data := make([]byte, size)

		n, err = f.Read(data)
		if err == io.EOF {
			return nil
		} else if uint32(n) != size {
			return errors.New("invalid data")
		}
		ch <- data
	}
}

func loopImport(ctx context.Context, dagChain *core.DagChain, unitCount int, ch <-chan []byte, done chan struct{}) {
	defer close(done)
	handleErr := func(err error) {
		utils.Fatalf("failed insert unit,error:%v", err)
	}
	var (
		count    int
		skipKown int
	)
	notify := func() {
		fmt.Printf("\rImported: %d Skip: %d Processed: %d%%", count, skipKown, (count+skipKown)*100/unitCount)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			unit, err := rawdb.DecodeRawUnit(data)
			if err != nil {
				handleErr(err)
			}
			err = dagChain.InsertUnit(unit)
			if err != nil {
				if err == consensus.ErrKnownBlock {
					skipKown++
					notify()
				} else {
					handleErr(err)
				}
			} else {
				count++
				notify()
			}
			if count+skipKown >= unitCount {
				return
			}
		}
	}
}
