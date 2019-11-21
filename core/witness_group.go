package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrAddrNodeIsExist  = errors.New("failed addrNode already exist")
	ErrAddrNodeNotExist = errors.New("failed addrNode not exit")
)

type AddrNodeType uint8

//go:generate enumer -type=AddrNodeType -json -transform=snake -trimprefix=AddrNodeType
const (
	AddrNodeTypeOffline      AddrNodeType = iota // 未连接
	AddrNodeTypeJoinUnderway                     // 连接中
	AddrNodeTypeStop                             // 见证人关闭
	AddrNodeTypeStart                            // 见证人开启
)

type Rlper interface {
	Rlp() ([]byte, error)
	UnRlp([]byte) error
}

type AddrNode struct {
	Addr      common.Address `json:"witness"`
	Node      *discover.Node
	Type      AddrNodeType `json:"status" rlp:"-"`
	PublicKey []byte       `json:"-" rlp:"-"` // 公钥
}

type addrNode struct {
	Addr      common.Address
	Node      string
	PublicKey []byte
}

func (a *AddrNode) EncodeRLP(write io.Writer) error {
	var node string
	if a.Node != nil {
		node = a.Node.String()
	}
	addrNode := addrNode{
		Addr:      a.Addr,
		Node:      node,
		PublicKey: a.PublicKey,
	}
	b, err := rlp.EncodeToBytes(&addrNode)
	if err != nil {
		return err
	}
	_, err = write.Write(b)
	if err != nil {
		return err
	}
	return nil
}

func (a *AddrNode) DecodeRLP(s *rlp.Stream) error {
	var addrNode addrNode
	if err := s.Decode(&addrNode); err != nil {
		return err
	}
	a.Addr = addrNode.Addr
	if len(addrNode.Node) != 0 {
		a.Node = discover.MustParseNode(addrNode.Node)
	}
	a.PublicKey = addrNode.PublicKey
	return nil
}

func NewAddrNode(address common.Address, node *discover.Node, t AddrNodeType, publicKey []byte) *AddrNode {
	addrNode := AddrNode{
		Addr:      address,
		Node:      node,
		Type:      t,
		PublicKey: publicKey,
	}
	return &addrNode
}

func (a *AddrNode) Rlp() ([]byte, error) {
	return rlp.EncodeToBytes(a)
}

func (a *AddrNode) UnRlp(data []byte) error {
	return rlp.DecodeBytes(data, a)
}

func (a *AddrNode) ValidNode(nodeID discover.NodeID) bool {
	if a.Node != nil && bytes.Equal(a.Node.ID.Bytes(), nodeID.Bytes()) {
		return true
	}
	return false
}

func (a *AddrNode) String() string {
	if a == nil {
		return ""
	}
	node := ""
	if a.Node != nil {
		node = a.Node.String()
	}
	return fmt.Sprintf("{addr:%v, node: %v, type: %v, publicKey: %v}", a.Addr.String(), node, a.Type, common.Bytes2Hex(a.PublicKey))
}

type AddrNodeList struct {
	list []AddrNode
	mux  sync.RWMutex `rlp:"-"`
}

func NewAddrNodeList() *AddrNodeList {
	return &AddrNodeList{
		list: make([]AddrNode, 0),
	}
}

func (an *AddrNodeList) Rlp() ([]byte, error) {
	return rlp.EncodeToBytes(an.List())
}

func (an *AddrNodeList) UnRlp(data []byte) error {
	return rlp.DecodeBytes(data, &an.list)
}

func (an *AddrNodeList) Len() int {
	return len(an.list)
}

func (an *AddrNodeList) Add(addrNode *AddrNode) error {
	an.mux.Lock()
	defer an.mux.Unlock()
	for _, v := range an.list {
		if v.Addr == addrNode.Addr {
			return ErrAddrNodeIsExist
		}
	}
	an.list = append(an.list, *addrNode)
	return nil
}

func (an *AddrNodeList) SetType(node discover.NodeID, t AddrNodeType) error {
	an.mux.Lock()
	defer an.mux.Unlock()
	for k, v := range an.list {
		if v.Node != nil && bytes.Compare(v.Node.ID.Bytes(), node.Bytes()) == 0 {
			an.list[k].Type = t
			return nil
		}
	}
	return ErrAddrNodeNotExist
}

func (an *AddrNodeList) Set(address common.Address, addrNode *AddrNode) {
	an.mux.Lock()
	defer an.mux.Unlock()
	for k, v := range an.list {
		if v.Addr == address {
			if addrNode.Node != nil {
				an.list[k].Node = addrNode.Node
			}
			an.list[k].Type = addrNode.Type
			return
		}
	}
}

func (an *AddrNodeList) Del(address common.Address) {
	an.mux.Lock()
	defer an.mux.Unlock()
	for k, v := range an.list {
		if v.Addr == address {
			an.list = append(an.list[:k], an.list[k+1:]...)
			return
		}
	}
}

func (an *AddrNodeList) Copy() *AddrNodeList {
	an.mux.RLock()
	defer an.mux.RUnlock()
	var addrNodeList AddrNodeList
	list := make([]AddrNode, len(an.list))
	copy(list, an.list)
	addrNodeList.list = list
	return &addrNodeList
}

func (an *AddrNodeList) List() []AddrNode {
	an.mux.RLock()
	defer an.mux.RUnlock()
	return an.list
}

func (an *AddrNodeList) Get(address common.Address) *AddrNode {
	an.mux.RLock()
	defer an.mux.RUnlock()
	for _, v := range an.list {
		if v.Addr == address {
			return &v
		}
	}
	return nil
}

func (an *AddrNodeList) GetAddrNodeForNodeId(nodeID discover.NodeID) *AddrNode {
	an.mux.RLock()
	defer an.mux.RUnlock()
	for _, v := range an.list {
		if v.Node != nil && bytes.Equal(v.Node.ID.Bytes(), nodeID.Bytes()) {
			return &v
		}
	}
	return nil
}

func (an *AddrNodeList) Empty() bool {
	an.mux.RLock()
	defer an.mux.RUnlock()
	if an == nil || len(an.list) == 0 {
		return true
	}
	return false
}
