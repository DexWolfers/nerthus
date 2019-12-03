package protocol

import (
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

type PeerEvent struct {
	Connected bool //如果已连接则为true
	PeerID    discover.NodeID
	Peer      Peer //当已连接时可以直接使用此发送消息
}

// 已连接节点接口
type Peer interface {
	AsyncSendMessage(code uint8, data interface{}) error
	Closed() bool
	ID() discover.NodeID
	SetWitnessType()
	Close(reason p2p.DiscReason)
	SendTransactions(txs types.Transactions) error
}

// 节点集操作接口，可操作当前集合中的节点
type PeerSet interface {
	// 查找给定ID的Peer，如果不存在则返回Nil
	Peer(id discover.NodeID) Peer
	// 获取所有已连接的Peer
	Peers() []Peer
	AddPeer(node *discover.Node) error
	// 从白名单中清理Peer
	ClearWhitelist(id discover.NodeID)
	//将Peer加入白名单
	JoinWhitelist(id discover.NodeID)
	//订阅Peer加入和退出事件
	SubscribePeerEvent(ch chan PeerEvent) event.Subscription
}
