// Author: @kulics
package types

import (
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

const (
	TRANSACTION = "transaction"
	UNIT        = "unit"
	VOTE        = "vote"

	START_WITNESS = "start_witness"
)

//go:generate stringer -type SyncMode

// SyncMode 同步模式
type SyncMode uint8

// TODO 增加同步某个hash之前的数据
const (
	MODE_FULL_BEGIN SyncMode = iota
	MODE_DIFF_BEGIN
	MODE_SINGLE_BEGIN
	MODE_SINGLE_BEHIND // 请求某一个单元后面所有单元
	//MODE_SINGLE_BEFORE // 请求 某个单元所处的链的范围[tailHash, hash]
	MODE_SINGLE_POINT_HASH
	MODE_SINGLE_POINT_NUMBER
)

type RealtimeSyncType = uint8

const (
	RealtimeSyncBad RealtimeSyncType = iota
	RealtimeSyncFuture
	RealtimeSyncBeta
)

//todo 加入abi需要的
type PendingLogsEvent struct {
	Logs []*Log
}

// TxPreEvent 当交易进入交易队列时发送
type TxPreEvent struct {
	IsLocal bool //是否是本地交易
	Tx      *Transaction
}

// NewWitnessedUnitEvent 新单元被见证
type NewWitnessedUnitEvent struct{ Unit *Unit }

// RemovedLogsEvent 交易日志移除事件
type RemovedLogsEvent struct{ Logs []*Log }

// FoundBadWitnessEvent 举报作恶事件
type FoundBadWitnessEvent struct {
	Reason    error
	Witness   common.Address
	Bad       Header        //作恶所在单元
	BadVotes  []WitenssVote //投票
	Good      Header        //相对作恶，所对标的正确单元
	GoodVotes []WitenssVote //投票
}

type ReportUnitResultEvent struct {
	Unit *Unit
}

// 传输交易
type PostedTransactionEvent struct {
	Data interface{}
}

// 暂停MC task
type PostedWitnessTask struct {
	MC common.Address
}

// 落地后传输单元
type PostedUnitEvent struct {
	Hash common.Hash
}

// 落地单元
type PostDagUnitEvent struct{ Unit *Unit }

// 同步单元
type PostSyncUnitEvent struct{ Unit *Unit }

// 传输投票
type PostVoteEvent struct{ Vote *VoteMsg }

// 报告有问题的交易哈希
type PostBadTxEvent struct {
	Tx common.Hash
}

type GetConsensusStateEvent struct {
	ChainAddress common.Address
	Ch           chan interface{}
}

// MissingAncestorsEvent 缺失祖先
type MissingAncestorsEvent struct {
	Chain   common.Address //链地址
	UHash   common.Hash    //所缺失的单元
	UNumber uint64         //所缺失的单元高度
}

// 同步请求信息
type SyncDagMsg struct {
	ClientID discover.NodeID `json:"-" rlp:"-"` // 客户端节点id，用来指定发回消息的目标，也就是请求方

	OriginClientID string           // 频率的限制，如果同一个节点多次请求同一个数据，则需要限速
	Type           SyncMode         // 响应类型
	Address        []common.Address // 查询的账号地址
	TargetHash     common.Hash      // 目标单元地址
	TargetNumber   uint64           // 目标单元位置

	Ehash common.Hash `json:"-" rlp:"-"` //测试使用

}

// 同步请求
type SyncDagRequestEvent struct {
	Data []byte
}

// 将内容广播
type SendBroadcastEvent struct {
	Data interface{}
	Code uint8
}

// 将内容定点发送
type SendDirectEvent struct {
	PeerID discover.NodeID
	Data   interface{}
	Code   uint8

	Ehash common.Hash `json:"-" rlp:"-"` //测试使用
	ToAll bool        `json:"-" rlp:"-"`
}

// 同步运行开关
type SyncCtrlEvent struct {
	Mode      uint8
	TrustPeer discover.NodeID
	Swt       bool
}

/// 全量同步事件（请求）
type FullSyncRequestEvent struct {
	Msg  interface{}
	Peer discover.NodeID
}

/// 全量同步时间（应答）
type FullSyncResponseEvent struct {
	Msg  interface{}
	Peer discover.NodeID
}

/// 新的广播事件事件
type NewBroadcastEvent struct {
	Msg  interface{}
	Peer discover.NodeID
}

/// 定向peers发送事件（只rlp一次）
type SendDirectPeersEvent struct {
	Code  uint8
	Msg   interface{}
	Peers []discover.NodeID
}

// 同步请求单个单元
type SyncGetEvent struct {
	Msg SyncDagMsg
}

type ChainEvent struct {
	Unit     *Unit       //单元(可能为空)
	UnitHash common.Hash //单元Hash
	Logs     []*Log
	Receipts Receipts
	Send     time.Time
}

// ChainVoteEvent 链单元投票信息
type ChainVoteEvent struct {
	Header      *Header
	NewVotes    Votes
	Before, Now int
}

type RealtimeSyncEvent struct {
	Unit     *Unit
	UnitHash common.Hash
	SyncType RealtimeSyncType
}

type ChainHeadEvent struct{ Unit *Unit }

// 系统链单元事件
type SystemChainUnitEvent struct {
}

type Status uint8

const (
	ChainNeedStop Status = iota + 1 //链已停止
	ChainCanStart                   //链已启动
	ChainActivate                   //链激活
)

// ChainMCProcessStatusChangedEvent 链操作处理状态改变事件
type ChainMCProcessStatusChangedEvent struct {
	MC     common.Address
	Status Status //
	Reason interface{}
}

// ReplayTxActionEvent 重发交易执行信息世界，在单元删除或分支切换时提供
type ReplayTxActionEvent struct {
	Tx *Transaction
}

type WitnessUserType uint8

const (
	_ WitnessUserType = iota

	//WitnessWorkChainAdd 见证人下新增加用户
	WitnessWorkChainAdd
	// WitnessWorkChainDrop 用户已经不是见证人用户
	WitnessWorkChainDrop
)

type WitnessWorkChainEvent struct {
	WitnessGroup uint64
	Chain        common.Address
	UID          UnitID //发送所在系统链单元
	Kind         WitnessUserType
}

type (
	WitnessWorkAction uint8

	// WitnessWorkEvent 见证人工作状态时间
	WitnessWorkEvent struct {
		Witness      common.Address
		WitnessGroup uint64
		Action       WitnessWorkAction
	}
)

const (
	_                  WitnessWorkAction = iota
	WitnessWorkStarted                   //已开启见证服务
	WitnessWorkStoped                    //已停止见证人服务
)

// 同组见证人连接状态
type WitnessConnectStatusEvent struct {
	Witness   common.Address
	Connected bool
}
