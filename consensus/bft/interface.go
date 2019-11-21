package bft

import (
	"context"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
	cprotocol "gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
)

// 将军选择器
type ProposalSelector func(set Validators, proposer common.Address, round, height uint64, preHash ...common.Hash) Validator

// 见证人容器接口
type ValidatorSet interface {
	Size() (s int)
	GetByIndex(idx uint64) Validator
	GetByAddress(address common.Address) (int, Validator)
	GetProposer() Validator
	IsProposer(address common.Address) bool
	IsNextProposer(address common.Address, round uint64) bool
	List() []Validator
	CalcProposer(proposer common.Address, round uint64)
	AddValidator(address common.Address) bool
	RemoveValidator(address common.Address) bool
	String() string
	Copy() ValidatorSet
	// 见证人数1/3取整
	F() int
	// 见证人数2/3取整
	TwoThirdsMajority() int
}

// 见证人接口
type Validator interface {
	Address() common.Address
	String() string
}

// 为共识引擎提供调用外部的接口
type Backend interface {
	// 本节点地址
	Address() common.Address

	// 获取见证人容器对象
	Validators(mc common.Address, header *types.Header) (ValidatorSet, error)

	// 给指定见证人地址发送消息
	DirectBroadcast(validator common.Address, msg protocol.MessageEvent, code uint8) error
	// 断开连接
	Disconnect(errMsg *protocol.Message, err error)

	// 见证人组内广播（包括自己）
	Broadcast(valSet ValidatorSet, msg protocol.MessageEvent) error

	// 给其他见证人广播消息
	Gossip(valSet ValidatorSet, msg protocol.MessageEvent) error

	// 提交最终共识结果
	Commit(proposal *MiningResult, vote []types.WitenssVote) error

	// 校验提案
	Verify(Proposal) (*MiningResult, error)

	// 签名
	Sign(sigHelper types.SignHelper) (types.SignHelper, error)

	// 检查签名
	CheckSignature(data []byte, addr common.Address, sig []byte) error

	// 获取最后一个稳定单元
	LastProposal(mc common.Address) (*types.Unit, common.Address)
	// 获取链尾单元头
	TailHeader(mc common.Address) (*types.Header, common.Address)

	// 获取创世hash
	GenesisHash() common.Hash

	// 获取指定高度的提案人
	GetProposer(mcaddr common.Address, number uint64) common.Address

	// 获取提案人
	GetParentProposer(childProposal Proposal) (common.Address, error)

	// 交易发送到交易池
	Rollback(unit Proposal)

	// 获取交易
	GetTransaction(txHash common.Hash) *types.Transaction

	// 计算单元间隔
	WaitGenerateUnitTime(parentNumber uint64, lastUnitStamp uint64) time.Duration
	// 广播链交易给组内见证人
	ReBroadTx(chain common.Address) int

	WitnessLib(chain common.Address, scHash common.Hash) []common.Address
}

// MiningAss 挖矿助手
type MiningAss interface {
	// 在新高度上打包一个新单元
	Mining(ctx context.Context, chain common.Address) (*MiningResult, error)
	// 校验新单元
	VerifyUnit(ctx context.Context, unit *types.Unit) (*MiningResult, error)
	//提交单元
	Commit(ctx context.Context, info *MiningResult) error
	// 获取广播对象
	Broadcaster() Broadcaster
	// 检查是否存在待处理交易
	ExistValidTx(mc common.Address) bool
	// 计算单元间隔
	WaitGenerateUnitTime(parentNumber uint64, lastUnitStamp uint64) time.Duration
	// 单元内交易回滚
	Rollback(unit *types.Unit)
	// 重新广播这条链的交易
	ReBroadTx(chain common.Address) int
}

// 交易池接口
type TxPool interface {
	AddRemote(*types.Transaction) error
}

// Broadcaster defines the interface to enqueue blocks to fetcher and find peer
type Broadcaster interface {
	// 获取指定见证人连接
	GetPeer(witness common.Address) cprotocol.Peer
	//广播消息给所有已连接的见证人
	Gossip(code uint8, msg interface{}) (peers int)
}

// Peer defines the interface to communicate with peer
type Peer interface {
	// Send sends the message to this peer
	AsyncSendMessage(code uint8, msg interface{}) error

	Close(reason p2p.DiscReason)
}
