// Author: @kulics
package core

import (
	"sync"

	"gitee.com/nerthus/nerthus/common/set"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/hashicorp/golang-lru"
)

// 管理dag模块
type DAG struct {
	db       ntsdb.Database // 数据库
	dagchain *DagChain
	eventMux *event.TypeMux // 可复用事件

	witnessLibCache      *lru.Cache //缓存指定位置的所有见证人清单
	witnessLibUpdateLock sync.Mutex
	badUnitCache         *set.StringSet
	arbitrationIngCache  *set.StringSet
}

// 创建一个新的dag把手
func NewDag(db ntsdb.Database, eventMux *event.TypeMux, dagchain *DagChain) (*DAG, error) {
	// 创建实例
	dag := &DAG{
		db:                  db,
		eventMux:            eventMux,
		badUnitCache:        set.NewStrSet(64, 0),
		arbitrationIngCache: set.NewStrSet(64, 0),
	}
	dag.witnessLibCache, _ = lru.New(20) //缓存20 个高度的见证人清单

	if dagchain != nil {
		dag.dagchain = dagchain
		// 创建dag时，将此信息告知chain
		dagchain.SetDag(dag)
	}
	dag.loadBadUnit()
	dag.loadArbitrationChains()
	return dag, nil
}

// 获取DagChain
// Author: @zwj
func (self *DAG) GetDagChain() *DagChain {
	return self.dagchain
}
