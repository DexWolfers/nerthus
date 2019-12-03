// Author: @ysqi
// 见证组直连管理
package witness

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"
	"sync/atomic"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	consensusProtocol "gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/statistics"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/rpc"
	"gitee.com/nerthus/nerthus/witness/service/wconn"
)

// Backend wraps all methods required for mining.
type Backend interface {
	AccountManager() *accounts.Manager
	EventMux() *event.TypeMux
	DagChain() *core.DagChain
	GetMainAccount() common.Address
	Genesis() *core.Genesis
	TxPool() *core.TxPool
	ChainDb() ntsdb.Database
	DagReader() *core.DAG
	CurrentIsSystemWitness() (bool, error)
	GetSystemWitnessList() ([]common.Address, error)
	ProtocolVersion() int
	PeerSet() protocol.PeerSet
	GetNode() *discover.Node
	GetSysWitList() (common.AddressList, error)
	GetPrivateKey() (*ecdsa.PrivateKey, error)
	Sta() *statistics.Statistic
	ServiceContext() *node.ServiceContext
	StopWitness() error
}

// 见证人消息的类型
type MsgWitnessType uint8

const (
	MsgWitnessTypeRun          MsgWitnessType = iota + 1 // 广播开启见证消息
	MsgWitnessTypeWitnessList                            // 见证组列表消息
	MsgWitnessTypeWitnessStart                           // 见证人开启消息
	MsgWitnessTypeWitnessStop                            // 见证人关闭消息
)

var (
	ErrNodeNotFind = errors.New("node not find or type different")
)

// 见证人消息体
type MsgStartWitness wconn.Message

// Witness creates blocks and searches for proof-of-work values.
type Witness struct {
	mux      *event.TypeMux
	schedule *WitnessSchedule

	mainAccount common.Address
	mining      int32 // 0:关闭 1:见证人开启 2:见证组连接
	nts         Backend

	conns *wconn.WitnessGroupConnControl

	canStart    int32 // can start indicates whether we can start the mining operation
	shouldStart int32 // should start indicates whether we should start after sync

	groupIndex uint64             // 当前见证人所在的见证组索引, 系统见证人为0
	addrNode   *core.AddrNodeList // 见证组列表和node关系列表
	msgTick    chan bool          // 是否广播开启见证人消息

	quit       chan struct{}
	privateKey *ecdsa.PrivateKey
	signer     types.Signer
}

// Author:@kulics 增加数据库handle
func New(nts Backend, mux *event.TypeMux, address common.Address,
	privateKey *ecdsa.PrivateKey, isSys bool) (*Witness, error) {
	var groupIndex uint64
	if !isSys {
		var err error
		groupIndex, err = nts.DagChain().GetWitnessGroupIndex(address)
		if err != nil {
			return nil, err
		}
	}

	//安全防范
	gotAddress, err := crypto.ParsePubKeyToAddress(privateKey.PublicKey)
	if err != nil {
		return nil, err
	}
	if gotAddress != address {
		return nil, fmt.Errorf("the address(%s)thas comes from private key is not same with input address(%s)",
			gotAddress, address)
	}
	//见证人节点集合

	miner := &Witness{
		nts:         nts,
		mux:         mux,
		canStart:    1,
		mainAccount: address,
		groupIndex:  groupIndex,
		addrNode:    core.NewAddrNodeList(),
		msgTick:     make(chan bool),
		privateKey:  privateKey,
		signer:      types.NewSigner(nts.DagChain().Config().ChainId),
	}
	if err := nts.ServiceContext().RegAPI(miner.APIs()...); err != nil {
		return nil, err
	}
	return miner, nil
}

// GetPrivateKey 获取当前见证人私钥
func (w *Witness) GetPrivateKey() *ecdsa.PrivateKey {
	return w.privateKey
}

// 获取链共识引擎状态
func (w *Witness) GetChainConsensusState(addr common.Address) (*bft.ConsensusInfo, error) {
	return w.schedule.minner.MiningStatus(addr), nil
}

// Start 开启见证
func (w *Witness) Start(chanWitnessOut chan<- struct{}, wnodeControl *wconn.WitnessGroupConnControl) error {
	if atomic.LoadInt32(&w.canStart) == 0 {
		return errors.New("Network syncing, will start miner afterwards")
	}
	if !atomic.CompareAndSwapInt32(&w.shouldStart, 0, 1) {
		return errors.New("witness already started")
	}
	if !atomic.CompareAndSwapInt32(&w.mining, 0, 1) {
		return errors.New("witness already started")
	}

	log.Info("Starting witness service", "witness", w.mainAccount)

	w.schedule = NewWitnessSchedule(w.nts, wnodeControl.ConnectedWitness())
	w.schedule.main = w
	w.conns = wnodeControl

	if err := w.schedule.Start(); err != nil {
		atomic.StoreInt32(&w.mining, 0)
		atomic.StoreInt32(&w.shouldStart, 0)
		return err
	}
	if err := wnodeControl.Start(&WConnHelp{w}); err != nil {
		return err
	}
	w.quit = make(chan struct{})

	log.Info("Started Witness service", "witness", w.mainAccount)
	return nil
}

// NeedWorkAt 检查当前见证人是否需要在给定链上进行见证工作
func (w *Witness) NeedWorkAt(chain common.Address) bool {
	if w.schedule == nil {
		return false
	}
	return w.schedule.MyIsUserWitness(chain) == nil
}

func (w *Witness) quitWitnessGroup(isWitness bool, witnessOup chan<- struct{}, cancelCh chan struct{}) bool {
	for _, v := range w.addrNode.List() {
		if v.Node != nil {
			w.nts.PeerSet().ClearWhitelist(v.Node.ID)
		}
	}
	w.addrNode = core.NewAddrNodeList()
	if !isWitness {
		witnessOup <- struct{}{}
		return true
	} else {
		atomic.StoreInt32(&w.mining, 1)
		close(cancelCh)
		return false
	}
}

// Stop 关闭见证
func (w *Witness) Stop() {
	if atomic.LoadInt32(&w.mining) == 0 {
		log.Debug("witness not started")
		return
	}
	close(w.quit)
	if w.conns != nil {
		w.conns.Stop()
	}
	w.schedule.Stop()
	atomic.StoreInt32(&w.mining, 0)
	atomic.StoreInt32(&w.shouldStart, 0)
	log.Info("stop witness", "witness", w.mainAccount)
}

// Mining 当前见证是否开启
func (w *Witness) Mining() bool {
	return atomic.LoadInt32(&w.mining) > 0
}

// GroupIndex 返回当前见证组索引
func (w *Witness) GroupIndex() uint64 {
	return w.groupIndex
}

// HandleConsensusMsg 处理共识消息
func (w *Witness) HandleConsensusMsg(ev consensusProtocol.MessageEvent) {
	if w.schedule != nil {
		w.schedule.handleConsensusMsg(ev)
	}
}
func (w *Witness) HandleMsg(peer discover.NodeID, msg *p2p.Msg) error {
	switch uint8(msg.Code) {
	case protocol.ChooseLastUnitMsg:
		if !w.Mining() {
			return errors.New("not mining")
		}
		if w.schedule.replaceWitnessWorker != nil {
			witness := w.conns.ConnectedWitness().GetWitnessById(peer)
			if witness.Empty() {
				return errors.New("not valid witness")
			}
			b, err := ioutil.ReadAll(msg.Payload)
			if err != nil {
				return err
			}
			return w.schedule.replaceWitnessWorker.HandleMsg(witness, b)
		}
	}
	return errors.New("can not handle")
}

// getPublicKey 获取公钥
func (w *Witness) getPublicKey(address common.Address) ([]byte, error) {
	info, err := w.nts.DagChain().GetWitnessInfo(address)
	if err != nil {
		return nil, err
	}
	if info.Status != sc.WitnessNormal {
		return nil, fmt.Errorf("witness status is %s", info.Status)
	}
	if info.GroupIndex != w.groupIndex {
		return nil, fmt.Errorf("witness is other group %d", info.GroupIndex)
	}
	return info.Pubkey, nil
}

// 是否是同组见证人
func (w *Witness) WitnessInGroup(witness common.Address) bool {
	if w.schedule == nil {
		return false
	}
	return w.schedule.ChainWitness().Have(witness)
}

// Schedule 返回见证模块
func (w *Witness) Schedule() *WitnessSchedule {
	return w.schedule
}

func (w *Witness) APIs() []rpc.API {
	return []rpc.API{
		{
			Namespace: "witness",
			Version:   "1.0",
			Service:   NewWitnessPublicAPI(w),
			Public:    true,
		},
	}
}

type WConnHelp struct {
	w *Witness
}

func (m *WConnHelp) WitnessGroup() uint64 {
	return m.w.schedule.curWitnessGroup
}

func (m *WConnHelp) Address() common.Address {
	return m.w.schedule.curWitness
}

func (m *WConnHelp) ENode() discover.Node {
	return *m.w.nts.GetNode()
}

func (m *WConnHelp) Sign(info types.SignHelper) error {
	return signThat(m.w.nts, info)
}

func (m *WConnHelp) Encrypt(pubkey []byte, data []byte) ([]byte, error) {
	account := accounts.Account{Address: m.Address()}
	wallet, err := m.w.nts.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	return wallet.Encrypt(pubkey, data)
}

func (m *WConnHelp) Decrypt(data []byte) ([]byte, error) {
	account := accounts.Account{Address: m.Address()}
	wallet, err := m.w.nts.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	buf, err := wallet.DecryptByPrivateKey(m.w.privateKey, data)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
