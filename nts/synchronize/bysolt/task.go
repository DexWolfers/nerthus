package bysolt

import (
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
)

func startSyncTasker(peer message.PeerID, backend Backend) *syncTasker {
	task := &syncTasker{
		peer:    peer,
		backend: backend,
		ch:      make(chan []common.Hash),
		seq:     new(int64),
	}
	go task.loop()
	return task
}

type syncTasker struct {
	peer    message.PeerID
	backend Backend
	ch      chan []common.Hash
	seq     *int64
}

func (t *syncTasker) add(hash []common.Hash) {
	t.ch <- hash
}
func (t *syncTasker) wait() {
	if t == nil || t.ch == nil {
		return
	}
	for len(t.ch) > 0 || atomic.LoadInt64(t.seq) > 0 {
		time.Sleep(time.Second)
	}
	if t.ch != nil {
		close(t.ch)
		t.ch = nil
	}
}
func (t *syncTasker) loop() {
	doneCh, sub := t.backend.SubChainEvent()
	defer sub.Unsubscribe()
	cache := make(map[common.Hash]struct{})
	for {
		select {
		case hashes, ok := <-t.ch: //发送任务
			if !ok {
				return
			}
			if len(hashes) == 0 {
				continue
			}
			if err := t.fetchUnits(hashes); err != nil {
				log.Trace("failed to fetch unit", "len", len(hashes), "first", hashes[len(hashes)-1], "err", err)
				continue
			}
			for _, hash := range hashes {
				if _, ok := cache[hash]; ok {
					continue
				}
				cache[hash] = struct{}{}
				atomic.AddInt64(t.seq, 1)
				//log.Trace("task of fetch miss unit", "hash", hash)
			}
		case ev := <-doneCh: // 接收完成
			if _, ok := cache[ev.UnitHash]; ok {
				delete(cache, ev.UnitHash)
				atomic.AddInt64(t.seq, -1)
				log.Trace("done task fetching miss uint", "hash", ev.UnitHash, "rest", len(cache))
			}
			if len(cache) == 0 {
				atomic.StoreInt64(t.seq, 0)
				return
			}
		case <-time.After(timeoutFetchUnit): // 不能无限等待下去
			// 释放wait
			atomic.StoreInt64(t.seq, 0)
			log.Debug("fetch miss unit timeout", "miss", len(cache))
			return
		}
	}
}

func (t *syncTasker) fetchUnits(hashes []common.Hash) error {
	for len(hashes) > 0 {
		length := maxFetchMisses
		if len(hashes) < length {
			length = len(hashes)
		}
		needSend := hashes[:length]
		if err := t.backend.FetchUnitByHash(needSend, t.peer); err != nil {
			if err := t.backend.FetchUnitByHash(needSend); err != nil {
				return err
			}
		}
		hashes = hashes[length:]
	}
	return nil
}

func (t *syncTasker) stop() {
	if atomic.LoadInt64(t.seq) > 0 {
		atomic.StoreInt64(t.seq, 0)
	}
}
