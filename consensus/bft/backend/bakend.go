package backend

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/bytebufferpool"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	ntsProtocol "gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/params"
)

func NewFaker() *backend {
	return &backend{
		fake: true,
	}
}

// New creates an backend for consensus core engine.
func New(config *bft.Config, emux *event.TypeMux, chain consensus.ChainReader, privateKey *ecdsa.PrivateKey, ass bft.MiningAss) consensus.Miner {

	backend := &backend{
		sign:              types.NewSigner(config.ChainId),
		config:            config,
		addressPrivateKey: privateKey,
		chain:             chain,
		coreStarted:       false,
		mining:            ass,
		logger:            log.New("module", "backend", "runMode", "witness"),
	}

	if privateKey != nil {
		backend.address = crypto.ForceParsePubKeyToAddress(privateKey.PublicKey)
		backend.logger = log.New("witness", backend.address.String())
	}
	if backend.mining != nil {
		backend.core = newEngineSchedule(backend, backend.config, backend.mining.Mining, backend.mining.ExistValidTx, config.ParallelChainLimit)
		//构建broadcaster
		backend.broadcaster = backend.mining.Broadcaster()
	}
	return backend
}

// ----------------------------------------------------------------------------
// 交易发送到交易池 func签名
type TxPoolFn = func(tx *types.Transaction)

// 共识调度引擎
// 连接共识引擎与链操作
type backend struct {
	sign              types.Signer
	config            *bft.Config
	address           common.Address
	addressPrivateKey *ecdsa.PrivateKey
	core              *engineSchedule
	logger            log.Logger
	chain             consensus.ChainReader
	coreStarted       bool
	coreMu            sync.RWMutex

	// event subscription for ChainHeadEvent event
	broadcaster bft.Broadcaster

	mining bft.MiningAss
	fake   bool

	bufferPool bytebufferpool.Pool
}

// 节点地址
func (sb *backend) Address() common.Address {
	return sb.address
}

// send message to a special node
// 给某个节点发送消息
func (sb *backend) DirectBroadcast(validator common.Address, msg protocol.MessageEvent, code uint8) error {
	p := sb.broadcaster.GetPeer(validator)
	if p == nil {
		return errors.New("missing witness connected peer")
	}
	payload, err := msg.Message.Payload()
	if err != nil {
		return err
	}
	return p.AsyncSendMessage(code, protocol.BFTMsg{ChainAddress: msg.Chain, Create: time.Now(), Payload: payload})
}

// disabled connect with witness peer
// 断开连接
func (sb *backend) Disconnect(errMsg *protocol.Message, err error) {
	p := errMsg.From
	sb.logger.Debug("disconnect witness peer", "peer", p.ID().String(), "err", err)
	p.Close(p2p.DiscUselessPeer)
}

// Broadcast message to all witness
// 见证人内广播
func (sb *backend) Broadcast(valSet bft.ValidatorSet, msg protocol.MessageEvent) error {

	// send to others
	if err := sb.Gossip(valSet, msg); err != nil {
		return err
	}
	msg.FromMe = true
	msg.PushTime = time.Now()
	sb.core.handleMsg(msg)
	return nil
}

// send message to all witness except me
// 给其他见证人广播消息
func (sb *backend) Gossip(valSet bft.ValidatorSet, msg protocol.MessageEvent) error {

	bStar := time.Now()
	if sb.broadcaster == nil {
		return errors.New("engine broadcaster is nil")
	}

	p, err := msg.Message.Payload()
	if err != nil {
		return err
	}
	//广播给所有已连接的见证人节点
	peers := sb.broadcaster.Gossip(ntsProtocol.BftMsg,
		protocol.BFTMsg{
			ChainAddress: msg.Chain,
			Payload:      p,
			Create:       time.Now(),
		})
	sb.logger.Trace("gossip message done", "chain", msg.Chain,
		"peers", peers, "msize", len(p), "cost", time.Now().Sub(bStar))

	majority := params.GetChainMinVoting(msg.Chain) - 1
	if peers < majority {
		return fmt.Errorf("connected witness not enough,online=%d least=%d", peers, majority)
	}
	return nil
}

// commit the unanimous result
// 提交共识达成的结果
func (sb *backend) Commit(proposal *bft.MiningResult, votes []types.WitenssVote) error {
	tr := tracetime.New().SetMin(0)
	defer tr.Stop()
	unit, ok := proposal.Unit.(*types.Unit)
	if !ok {
		return errInvalidProposal
	}
	tr.Tag().AddArgs("uhash", unit.Hash())
	unit = unit.WithBody(unit.Transactions(), votes, unit.Sign)
	proposal.Unit = unit
	tr.Tag().AddArgs("chain", unit.MC(), "number", unit.Number(), "proposer", unit.Proposer())
	logger := sb.logger.New("chain", unit.MC(), "number", unit.Number(), "unit", unit.Hash(), "proposer", unit.Proposer())
	if err := sb.mining.Commit(context.Background(), proposal); err != nil {
		tr.AddArgs("status", err)
		logger.Error("Committed Unit Failed", "error", err)
	} else {
		tr.AddArgs("status", "ok")
		logger.Debug("Committed Unit")
	}
	return nil
}

// Verify proposal
// 校验提案
func (sb *backend) Verify(proposal bft.Proposal) (*bft.MiningResult, error) {
	// Check if the proposal is a valid block
	tr := tracetime.New()
	defer tr.Stop()
	block, ok := proposal.(*types.Unit)
	tr.AddArgs("uhash", block.Hash())
	if !ok {
		sb.logger.Error("Invalid proposal, %v", proposal)
		return nil, errInvalidProposal
	}
	tr.Tag()
	return sb.mining.VerifyUnit(context.Background(), block)
}

// sign
// 签名
func (sb *backend) Sign(sigHelper types.SignHelper) (types.SignHelper, error) {
	return sigHelper, types.SignBySignHelper(sigHelper, sb.sign, sb.addressPrivateKey)
}

// verify signed
// 校验签名
func (sb *backend) CheckSignature(data []byte, address common.Address, sig []byte) error {
	signer, err := bft.GetSignatureAddress(data, sig)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return err
	}
	// Compare derived addresses
	if signer != address {
		return errInvalidSignature
	}
	return nil
}

// get Genesis hash
// 取创世hash
func (sb *backend) GenesisHash() common.Hash {
	return sb.chain.GenesisHash()
}

// get owner of proposal by given number
// 获取指定高度的提案人
func (sb *backend) GetProposer(chain common.Address, number uint64) common.Address {
	if unit := sb.chain.GetUnitByNumber(chain, number); unit != nil {
		a, _ := sb.Author(unit)
		return a
	}
	return common.Address{}
}

// Deprecated: useless func
// 获取旧提案的提案人
func (sb *backend) GetParentProposer(proposal bft.Proposal) (common.Address, error) {
	unit := proposal.(*types.Unit)
	signer := types.NewSigner(sb.chain.Config().ChainId)
	return types.Sender(signer, unit)
}

// get last stable proposal unit
// 获取最后一个稳定单元
func (sb *backend) LastProposal(mc common.Address) (*types.Unit, common.Address) {
	block := sb.chain.GetUnitByHash(sb.chain.GetChainTailHash(mc))

	var proposer common.Address
	if block.Number() > 0 {
		var err error
		proposer = block.Proposer()
		if err != nil {
			sb.logger.Error("Failed to get block proposer", "err", err)
			return nil, common.Address{}
		}
	}
	// Return header only block here since we don't need block body
	return block, proposer
}

// 取链尾hash
func (sb *backend) TailHeader(mc common.Address) (*types.Header, common.Address) {
	header := sb.chain.GetChainTailHead(mc)
	return header, header.Proposer
}

// send transaction to tx pools
// 交易发送到交易池
func (sb *backend) Rollback(unit bft.Proposal) {
	sb.mining.Rollback(unit.(*types.Unit))
}

// get transaction by given hash
// 根据hash查交易
func (sb *backend) GetTransaction(txHash common.Hash) *types.Transaction {
	return sb.chain.GetTransaction(txHash)
}

// 计算单元间隔
func (sb *backend) WaitGenerateUnitTime(parentNumber uint64, lastUnitStamp uint64) time.Duration {
	return sb.mining.WaitGenerateUnitTime(parentNumber, lastUnitStamp)
}

// 广播链交易给组内见证人
func (sb *backend) ReBroadTx(chain common.Address) int {
	return sb.mining.ReBroadTx(chain)
}

func (sb *backend) WitnessLib(chain common.Address, scHash common.Hash) []common.Address {
	list, _ := sb.chain.GetChainWitnessLib(chain, scHash)
	return list
}
