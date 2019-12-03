package arbitration

import (
	"math"
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

func PrepareNodesForTest(t *testing.T, commitCh chan common.Hash) []*core {
	testutil.SetupTestLogging()
	// 准备理事地址
	councils := make([]common.Address, 0)
	for i := 1; i <= 11; i++ {
		councils = append(councils, common.BytesToAddress([]byte{byte(i)}))
	}

	nodes := make([]*core, 0)
	// 模拟节点
	for _, addr := range councils {
		c := newCore(addr, &TestBackend{
			councils,
			3,
			commitCh,
			func(msg *CouncilMessage) {
				for i := range nodes {
					nodes[i].HandleVote(msg)
				}
			},
		}, func() {
		})
		c.Start()
		nodes = append(nodes, c)
	}
	return nodes
}
func verifyResult(t *testing.T, nodes []*core, commitCh chan common.Hash) {
	var result common.Hash
	for i := 0; i < len(nodes); i++ {
		r := <-commitCh
		if result.Empty() {
			result = r
		} else {
			if r != result {
				t.Fail()
			}
		}
		log.Info(">>>>>>>>> commit ", "hash", r)
	}
}

func TestVoteNormal(t *testing.T) {
	commitCh := make(chan common.Hash)
	nodes := PrepareNodesForTest(t, commitCh)
	//全部激活
	for i := range nodes {
		nodes[i].Active()
	}
	verifyResult(t, nodes, commitCh)
}
func TestActive31(t *testing.T) {
	commitCh := make(chan common.Hash)
	nodes := PrepareNodesForTest(t, commitCh)
	//激活1/3+1
	for i := range nodes {
		if i > len(nodes)/3+1 {
			break
		}
		nodes[i].Active()
	}

	verifyResult(t, nodes, commitCh)
}
func TestArbitration(t *testing.T) {
	commitCh := make(chan common.Hash)
	nodes := PrepareNodesForTest(t, commitCh)
	// 1/3+1 超时激活，剩余部分收到了单元
	for i := range nodes {
		if i < int(math.Ceil(float64(len(nodes))*2/3))-1 {
			nodes[i].backend.(*TestBackend).scNubmer++
		} else {
			nodes[i].Active()
		}
	}
	verifyResult(t, nodes[:int(math.Ceil(float64(len(nodes))*2/3))-1], commitCh)
	log.Info("reset")
	nodes = PrepareNodesForTest(t, commitCh)
	// 超过1/2 超时激活
	for i := range nodes {
		if i < int(math.Ceil(float64(len(nodes))/2)-1) {
			nodes[i].backend.(*TestBackend).scNubmer++
		} else {
			nodes[i].Active()
		}
	}
	verifyResult(t, nodes[len(nodes)/2:], commitCh)

}
func TestArbitrationHalf(t *testing.T) {
	commitCh := make(chan common.Hash)
	nodes := PrepareNodesForTest(t, commitCh)
	// 各一半的僵局
	nodes = PrepareNodesForTest(t, commitCh)
	for i := range nodes {
		if i < len(nodes)/2 {
			nodes[i].backend.(*TestBackend).scNubmer++
		} else {
			nodes[i].Active()
		}
	}
	verifyResult(t, nodes[:len(nodes)/2], commitCh)
}
func TestDishonest(t *testing.T) {
	commitCh := make(chan common.Hash)
	nodes := PrepareNodesForTest(t, commitCh)
	bad := nodes[3]
	bad.Active()
	pro, _ := bad.backend.GenerateProposal(bad.backend.ScTail())
	msg1 := &CouncilMessage{
		From:   bad.address,
		ScTail: bad.scTail,
		Dist:   pro.Hash(),
		Status: bad.state,
	}
	msg2 := &CouncilMessage{
		From:   bad.address,
		ScTail: bad.scTail,
		Dist:   bad.backend.ScTail().Hash(),
		Status: bad.state,
	}
	log.Debug(">>>>>dishonest", "hash1", msg1.Dist, "hash2", msg2.Dist)
	bad.backend.Broadcast(msg1)
	bad.backend.Broadcast(msg2)
	<-time.After(time.Second * 10)
}
func TestPublicAddress(t *testing.T) {
	key, err := crypto.HexToECDSA(params.PublicPrivateKey)
	if err != nil {
		t.Error(err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	t.Log(addr.Hex())
}

// 1000理事投票，单元size
func TestUnitSize(t *testing.T) {
	// create unit

	header := &types.Header{
		MC:         params.SCAccount,
		Number:     1,
		ParentHash: common.HexToHash("0x9523a1cdcc27b8940aa229fe76ee73592f27ea717917d8a38a49ea54c7bf8f8b"),
		SCHash:     common.HexToHash("0x9523a1cdcc27b8940aa229fe76ee73592f27ea717917d8a38a49ea54c7bf8f8b"),
		SCNumber:   1,
		Proposer:   params.PublicPrivateKeyAddr,
		Timestamp:  uint64(time.Now().UnixNano()),
		GasLimit:   800000,
	}
	// 使用一个公开的秘钥签名
	prkey, err := crypto.HexToECDSA(params.PublicPrivateKey)
	signer := types.NewSigner(big.NewInt(1280))
	if err != nil {
		t.Error(err)
	}
	tx := types.NewTransaction(params.PublicPrivateKeyAddr, params.SCAccount, big.NewInt(0), 0,
		0, types.DefTxTimeout, sc.CreateCallInputData(sc.FuncIdAllSystemWitnessReplace))
	err = types.SignBySignHelper(tx, signer, prkey)
	if err != nil {
		t.Error(err)
	}
	exec := &types.TransactionExec{
		Action: types.ActionSCIdleContractDeal,
		TxHash: tx.Hash(),
		Tx:     tx,
	}
	txs := []*types.TransactionExec{exec}
	unit := types.NewUnit(header, txs)
	err = types.SignBySignHelper(unit, signer, prkey)
	if err != nil {
		t.Error(err)
	}

	// include votes
	dist := common.HexToHash("0xd6284ae4c3294d4e72ad3a3ebb51c7107ff5e182066b823c7a519949d76e06d3")
	seals := make([]types.WitenssVote, 0)
	for i := 0; i < 10000; i++ {
		msg := &CouncilMessage{ScTail: 10000000000, Dist: dist, Status: StatePrepare}
		err = types.SignBySignHelper(msg, signer, prkey)
		if err != nil {
			t.Error(err)
		}
		v := types.WitenssVote{msg.Extra(), msg.Sign}
		seals = append(seals, v)
	}
	t.Logf("votes:%d", len(seals))
	// build unit
	unit = unit.WithBody(unit.Transactions(), seals, unit.Sign)
	t.Logf("unit size:%s int64:%d", unit.Size().String(), unit.Size().Int64())
	if unit.Size().Int64() >= 1024*1024 {
		t.Fail()
	}
}

type TestProposal struct {
	id   string
	hash common.Hash
}

func (t *TestProposal) Hash() common.Hash {
	return t.hash
}

type TestBackend struct {
	couns    []common.Address
	scNubmer uint64
	commit   chan common.Hash
	broad    func(msg *CouncilMessage)
}

func (t *TestBackend) GenerateProposal(scTail *types.Header) (Proposal, error) {
	n := "black"
	return &TestProposal{
		n,
		common.BytesToHash([]byte(n)),
	}, nil
}
func (t *TestBackend) GetCouncils() ([]common.Address, error) {
	return t.couns, nil
}
func (t *TestBackend) Commit(pro Proposal, seal []types.WitenssVote) error {
	t.commit <- pro.Hash()
	return nil
}
func (t *TestBackend) ScTail() *types.Header {
	return &types.Header{
		Number: t.scNubmer,
	}
}

func (t *TestBackend) GetUnitByHash(hash common.Hash) *types.Unit {
	return nil
}
func (t *TestBackend) Sign(sign types.SignHelper) error {
	return nil
}
func (t *TestBackend) VerifySign(sign types.SignHelper) (common.Address, error) {
	return sign.(*CouncilMessage).From, nil
}
func (t *TestBackend) Broadcast(msg *CouncilMessage) {
	go t.broad(msg)
}
