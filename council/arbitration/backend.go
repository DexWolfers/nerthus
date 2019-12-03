package arbitration

import (
	"crypto/ecdsa"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	core2 "gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	protocol2 "gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/params"
	"github.com/pkg/errors"
)

func newBackend(dag *core2.DagChain, signer types.Signer, privKey *ecdsa.PrivateKey, eventMux *event.TypeMux) *backend {
	return &backend{
		dag, signer, privKey, eventMux,
	}
}

type backend struct {
	dag               *core2.DagChain
	sign              types.Signer
	addressPrivateKey *ecdsa.PrivateKey
	eventMux          *event.TypeMux
}

// 打包特殊单元
func (t *backend) GenerateProposal(scTail *types.Header) (Proposal, error) {
	tailHash := scTail.Hash()

	// create header
	uTime := scTail.Timestamp + uint64((params.SCDeadArbitrationInterval * time.Second).Nanoseconds())
	header := &types.Header{
		MC:         params.SCAccount,
		Number:     scTail.Number + 1,
		ParentHash: tailHash,
		SCHash:     tailHash, // 系统链稳定单元
		SCNumber:   scTail.Number,
		Proposer:   params.PublicPrivateKeyAddr,
		Timestamp:  uTime, //时间戳=父单元时间戳+停摆检查时间config
		GasLimit:   t.dag.Config().Get(params.ConfigParamsUcGasLimit),
	}
	// 使用一个公开的秘钥签名
	prkey, err := crypto.HexToECDSA(params.PublicPrivateKey)
	if err != nil {
		return nil, err
	}
	// create tx
	statedb, err := t.dag.StateAt(params.SCAccount, scTail.StateRoot)
	if err != nil {
		return nil, err
	}
	txs, err := t.generateReplaceTx(prkey, header, statedb, uTime)
	if err != nil {
		return nil, err
	}
	log.Debug("include tx to proposal", "txhash", txs[0].TxHash, "uhash", header.Hash())
	// create unit
	unit := types.NewUnit(header, txs)
	err = types.SignBySignHelper(unit, t.sign, prkey)
	if err != nil {
		return nil, err
	}
	return unit, nil

}

// 获取理事
func (t *backend) GetCouncils() ([]common.Address, error) {
	statedb, err := t.dag.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	return sc.GetCouncilList(statedb)
}
func (t *backend) Commit(pro Proposal, seal []types.WitenssVote) error {
	unit := pro.(*types.Unit)
	unit = unit.WithBody(unit.Transactions(), seal, unit.Sign)
	return t.dag.InsertUnit(unit)
}
func (t *backend) ScTail() *types.Header {
	return t.dag.GetChainTailHead(params.SCAccount)
}
func (t *backend) GetUnitByHash(hash common.Hash) *types.Unit {
	return t.dag.GetUnitByHash(hash)
}
func (t *backend) Sign(sign types.SignHelper) error {
	return types.SignBySignHelper(sign, t.sign, t.addressPrivateKey)
}
func (t *backend) VerifySign(sign types.SignHelper) (common.Address, error) {
	return types.Sender(t.sign, sign)
}
func (t *backend) Broadcast(msg *CouncilMessage) {
	msg.TimeStamp = uint64(time.Now().UnixNano())
	t.eventMux.Post(&types.SendBroadcastEvent{
		Data: CouncilMsgEvent{Message: msg},
		Code: protocol2.CouncilMsg,
	})
}

func (t *backend) generateReplaceTx(priKey *ecdsa.PrivateKey, header *types.Header, db *state.StateDB, timeStamp uint64) ([]*types.TransactionExec, error) {
	tx := types.NewTransaction(params.PublicPrivateKeyAddr, params.SCAccount, big.NewInt(0), 0,
		0, types.DefTxTimeout, sc.CreateCallInputData(sc.FuncIdAllSystemWitnessReplace))
	tx.SetTimeStamp(timeStamp)
	err := types.SignBySignHelper(tx, t.sign, priKey)
	if err != nil {
		return nil, err
	}
	exec := &types.TransactionExec{
		Action: types.ActionSCIdleContractDeal,
		TxHash: tx.Hash(),
		Tx:     tx,
	}
	totalUsedGas := new(big.Int)
	gp := new(core2.GasPool).AddGas(new(big.Int).SetUint64(header.GasLimit))

	db.Prepare(tx.Hash(), common.Hash{}, 0, header.Number)
	receipt, err := core2.ProcessTransaction(t.dag.Config(), t.dag, db,
		header, gp, exec, totalUsedGas, vm.Config{}, nil)
	if err != nil {
		return nil, err
	}
	if receipt.Failed {
		return nil, errors.New("create replace tx failed")
	}
	log.Debug("generate replace all sys tx", "receipt logs", len(receipt.Logs))
	receipts := types.Receipts{receipt}
	// 插入部分结果数据
	header.StateRoot, _ = db.IntermediateRoot(header.MC)
	header.GasUsed = totalUsedGas.Uint64()
	header.ReceiptRoot = types.DeriveSha(receipts)
	header.Bloom = types.CreateBloom(receipts)
	return []*types.TransactionExec{exec}, nil
}
