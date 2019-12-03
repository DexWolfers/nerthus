package wconn

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/sync/cmap"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/metrics"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

//go:generate gencodec -type WitnessNode -field-override witnessNodeMarshaling -out gen_witnessNode_json.go

type WitnessNode struct {
	types.SignContent //签名信息

	Version uint64         `json:"version"   gencodec:"required"` //更新版本，只有在变化时才需要修改，由自己维护
	Enode   discover.Node  `json:"node"	  gencodec:"required"`    //网络连接信息
	Witness common.Address `json:"witness"   gencodec:"required"` //见证人

	UpdateTime time.Time `json:"updateTime" rlp:"-"` //不属于 rlp ,也不属于签名内容

	hash atomic.Value
}

func (n *WitnessNode) String() string {
	return fmt.Sprintf("%s:v%d@%s", n.Witness.String(), n.Version, n.Enode.String())
}

func (n *WitnessNode) Str() string {
	return fmt.Sprintf("%s:v%d@%s", n.Witness.Str(), n.Version, n.Enode.String())
}

type witnessNodeMarshaling struct {
	Hash common.Hash `json:"hash"`
}

type nodrInfoRlp struct {
	Version uint64
	Enode   string
	Witness common.Address
	Sign    []byte
}

func (node *WitnessNode) DecodeRLP(s *rlp.Stream) error {
	info := new(nodrInfoRlp)
	if err := s.Decode(info); err != nil {
		return err
	}
	enode, err := discover.ParseNode(info.Enode)
	if err != nil {
		return err
	}

	node.Witness = info.Witness
	node.Enode = *enode
	node.Version = info.Version
	node.SetSign(info.Sign)
	return nil
}

func (node *WitnessNode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, nodrInfoRlp{
		Version: node.Version,
		Enode:   node.Enode.String(),
		Witness: node.Witness,
		Sign:    node.GetSig(),
	})
}

func (node *WitnessNode) GetSignStruct() []interface{} {
	return []interface{}{
		node.Version, node.Enode,
		node.Witness,
	}
}

func (node *WitnessNode) Hash() common.Hash {
	if v := node.hash.Load(); v != nil {
		return v.(common.Hash)
	}
	hash := common.SafeHash256(node.GetSignStruct())
	node.hash.Store(hash)
	return hash
}

//go:generate enumer -type=WitnessNodeStatus -json -transform=snake
type WitnessNodeStatus uint8

const (
	Unknown       WitnessNodeStatus = iota //未知，表示并不清楚见证人连接信息
	TryConnecting                          //尝试连接中
	NetConnected                           //已连接，但是还在检查中
	Joined                                 //已完成握手，加入
	Disconnected                           //已断开
)

type WitnessInfo struct {
	Witness  common.Address    `json:"witness"`
	NodeInfo *WitnessNode      `json:"node"`
	PubKey   []byte            `json:"-"`
	Status   WitnessNodeStatus `json:"status"`

	//连接信息
	ConnectedTime time.Time `json:"connectedTime,omitempty"`
	JoinTime      time.Time `json:"joinTime,omitempty"` //当前连接时间

	PrevDisconnect   time.Time              `json:"-"` //上次退出时间
	PrevJoinDuration time.Duration          `json:"-"` //上次连接时长
	AvgConnDuration  *metrics.StandardTimer `json:"-"` //自连接以来，每次连接平均时长
}

func (w *WitnessInfo) SetStatus(logger log.Logger, status WitnessNodeStatus) {
	if w.Status == status {
		return
	}
	if logger == nil {
		logger = log.Root()
	}

	logf := logger.Trace
	switch status {
	case Joined, NetConnected, Disconnected:
		logf = logger.Info
	}
	logf("witness node connect status changed",
		"witness", w.Witness, "before", w.Status, "now", status,
		"wnode", w.NodeInfo)

	old := w.Status
	w.Status = status
	now := time.Now()
	switch status {
	case NetConnected:
		w.ConnectedTime = time.Now()
		//如果已经是 Joined 状态，则不修改
		if old == Joined {
			w.Status = Joined
		} else {
			w.JoinTime = time.Time{}
		}
	case Joined:
		w.JoinTime = time.Now()
		if w.ConnectedTime.IsZero() {
			w.ConnectedTime = w.JoinTime
		}
	case Disconnected, TryConnecting:
		if !w.JoinTime.IsZero() {
			dur := now.Sub(w.JoinTime)
			w.AvgConnDuration.UpdateSince(now)

			w.PrevJoinDuration = dur
			w.PrevDisconnect = now
		} else {
			w.PrevJoinDuration = 0
			w.PrevDisconnect = time.Time{}
		}

		w.ConnectedTime = time.Time{}
		w.JoinTime = time.Time{}
	}
}
func NewWintessInfo(w common.Address, pubkey []byte) *WitnessInfo {
	return &WitnessInfo{
		Witness:         w,
		PubKey:          pubkey,
		Status:          Unknown,
		AvgConnDuration: metrics.NewStandardTimer(),
	}
}

type WitnessMap struct {
	infos   cmap.ConcurrentMap
	nodeIds sync.Map

	MeInfo *WitnessNode
	logger log.Logger
}

type ActionLog struct {
	Time    time.Time
	Witness common.Address
	Ctx     []interface{}
}

func NewWitnessMap(logger log.Logger) *WitnessMap {
	return &WitnessMap{
		infos:   cmap.New(),
		logger:  logger,
		nodeIds: sync.Map{},
	}
}

func (wm *WitnessMap) Load(witness common.Address) *WitnessInfo {
	v, ok := wm.infos.Get(witness.String())
	if !ok {
		return nil
	}
	return v.(*WitnessInfo)
}
func (wm *WitnessMap) Store(info *WitnessInfo) {
	wm.infos.Set(info.Witness.String(), info)
	wm.StoreNodeKey(info)
}
func (wm *WitnessMap) StoreNodeKey(info *WitnessInfo) {
	if info == nil || info.NodeInfo == nil {
		return
	}
	wm.nodeIds.Store(info.NodeInfo.Enode.ID, info.Witness)
}

//删除记录的节点信息
func (wm *WitnessMap) DelNode(id discover.NodeID) {
	wm.nodeIds.Delete(id)
}

func (wm *WitnessMap) LoadById(id discover.NodeID) *WitnessInfo {
	addr, ok := wm.nodeIds.Load(id)
	if !ok {
		return nil
	}
	return wm.Load(addr.(common.Address))
}

func (wm *WitnessMap) All() []*WitnessInfo {
	list := make([]*WitnessInfo, 0, params.ConfigParamsUCWitness)
	wm.infos.IterCb(func(key string, v interface{}) bool {
		list = append(list, v.(*WitnessInfo))
		return true
	})
	return list
}

// 清理所有连接信息
func (wm *WitnessMap) Clearn() {
	for _, k := range wm.infos.Keys() {
		wm.infos.Remove(k)
	}
	wm.nodeIds.Range(func(key, value interface{}) bool {
		wm.nodeIds.Delete(key)
		return true
	})
}

func (wm *WitnessMap) Len() int {
	return wm.infos.Count()
}
