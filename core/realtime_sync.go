package core

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

const (
	graphSyncFutureQueueRandomN = 20
)

type RealTimeSync interface {
	SyncUnit(unitSyncEvent types.RealtimeSyncEvent)
	SyncGraph(nodeName common.Hash) string
	ZeroNodes() []common.Hash
	Stop()
}

type RealtimeSyncV2 struct {
	dc *DagChain
}

func NewRealtimeSyncV2(dc *DagChain) *RealtimeSyncV2 {
	r := RealtimeSyncV2{
		dc: dc,
	}
	return &r
}
func (r *RealtimeSyncV2) SyncUnit(ev types.RealtimeSyncEvent) {
	if r.dc.dag != nil && r.dc.dag.eventMux != nil {
		r.dc.unitStatusFeed.Send(ev)
	}
}

func (r *RealtimeSyncV2) SyncGraph(nodeName common.Hash) string {
	return ""
}
func (r *RealtimeSyncV2) ZeroNodes() []common.Hash {
	return nil
}
func (r *RealtimeSyncV2) Stop() {

}
