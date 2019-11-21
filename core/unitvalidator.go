package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/ethash"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

var (
	errNotNilTransaction = errors.New("transaction should be not nil")
	errNilTransaction    = errors.New("transaction should be nil")
	errNilRecipient      = errors.New("recipient is nil")
	errNotDefineAction   = errors.New("not defined action")
	errNotEmptySource    = errors.New("source hash should not be empty")
)

// UnitValidator is responsible for validating unit headers, uncles and
// processed state.
//
// UnitValidator implements Validator.
type UnitValidator struct {
	config         *params.ChainConfig // Chain configuration options
	bc             *DagChain           // Canonical unit chain
	engine         consensus.Engine    // Consensus engine used for validating
	validWithCache func(tx *types.Transaction) error
	signer         types.Signer
}

// NewUnitValidator returns a new unit validator which is safe for re-use
func NewUnitValidator(config *params.ChainConfig, unitchain *DagChain, engine consensus.Engine) *UnitValidator {
	validator := &UnitValidator{
		config: config,
		engine: engine,
		bc:     unitchain,
		signer: types.MakeSigner(*config),
	}
	return validator
}

func (v *UnitValidator) validateTx(header *types.Header, tx *types.TxExecs) error {
	// 判断是否双花，需要只需要保证在同一条链上不重复出现，
	// 而不能依靠高度判断，原因是出单元逻辑变更
	//dagchain := v.bc

	//var txSender common.Address
	////var to common.Address
	//var err error
	//
	//if !tx.PreStep.IsEmpty() && tx.Tx != nil {
	//	return errors.New("invalid source or tx field")
	//}
	//
	//if !tx.PreStep.IsEmpty() {
	//	stable := v.bc.GetHeaderByNumber(tx.PreStep.ChainID, tx.PreStep.Height)
	//	if stable == nil {
	//		log.Warn("the source unit is not stabled now",
	//			"chain", header.MC, "number", header.Number, "uhash", header.Hash(), "txhash", tx.TxHash, "source", tx.PreStep)
	//		return consensus.ErrUnknownAncestor
	//	}
	//	if stable.Hash() != tx.PreStep.Hash {
	//		return errors.New("the source unit is not stabled")
	//	}
	//	if stable.Timestamp >= header.Timestamp {
	//		return fmt.Errorf("timestamp %d must be greater than prestep's %d",
	//			header.Timestamp, stable.Timestamp)
	//	}
	//}

	//log.Debug("=======> tx action ==>", "action", tx.Action)
	//switch tx.Action {
	//default:
	//	return errNotDefineAction
	//case types.ActionTransferPayment, types.ActionContractFreeze:
	//	// 检查单元广播者是不是交易方的见证人
	//	if tx.Tx == nil {
	//		return errNilTransaction
	//	}
	//
	//	if !tx.PreStep.IsEmpty() {
	//		return errors.New("source hash should  be empty")
	//	}
	//
	//	if txSender != header.MC {
	//		return ErrToMc
	//	}
	//
	//	if tx.Action == types.ActionContractFreeze {
	//		if !to.IsContract() {
	//			return errors.New("invalid address:to should be contract address")
	//		}
	//	}
	//	// 只允许有一条或者零条，如果有则必须保证to=tx.to
	//	if len(tx.Result.Coinflows) > 1 ||
	//		(len(tx.Result.Coinflows) == 1 && tx.Result.Coinflows[0].To != tx.Tx.To()) {
	//		return errors.New("invalid coinflows: need contains receipt info")
	//	}
	//case types.ActionTransferReceipt:
	//	if tx.PreStep.IsEmpty() {
	//		return errNotEmptySource
	//	}
	//
	//	origTxEx, err := dagchain.dag.TxExecInfo(tx.PreStep.Hash, tx.TxHash)
	//	if err != nil {
	//		if err == ErrUnitNotFind { //单元不存在
	//			return consensus.ErrUnknownAncestor
	//		}
	//		return err
	//	}
	//
	//	if tx.Tx != nil {
	//		return errNotNilTransaction
	//	}
	//
	//	// 判断交易接受方是否为同一个人
	//	recipient := origTxEx.Tx.To()
	//
	//	if recipient != header.MC {
	//		return ErrToMc
	//	}
	//
	//	// 校验coinflow
	//	//log.Debug("====== coinflow: ========>", "len", len(tx.Result.Coinflows), "to", tx.Result.Coinflows[0].To.String(), "mc", header.MC.String())
	//
	//	// 只允许有一条或者零条，如果有则必须保证to=mc
	//	if len(tx.Result.Coinflows) > 1 ||
	//		(len(tx.Result.Coinflows) == 1 && tx.Result.Coinflows[0].To != header.MC) {
	//		return errors.New("invalid coinflows: need contains receipt info")
	//	}
	//
	//case types.ActionPowTxContractDeal:
	//	if tx.Tx == nil {
	//		return errNilTransaction
	//	}
	//
	//	if to != params.SCAccount {
	//		return errors.New("invalid address:'to' should be system account")
	//	}
	//
	//	if header.MC != params.SCAccount {
	//		return ErrToMc
	//	}
	//	// 特殊合约校验
	//	funcId := common.Bytes2Hex(tx.Tx.Data4())
	//	switch funcId {
	//	case sc.FuncIdAllSystemWitnessReplace: // 必须出现在特殊单元中
	//		if header.Proposer != params.PublicPrivateKeyAddr {
	//			return errors.New("invalid host unit:should be special unit")
	//		}
	//	}
	//
	//case types.ActionSCIdleContractDeal:
	//	// 空单元处理
	//	if tx.Tx == nil {
	//		return errNilTransaction
	//	}
	//
	//	if !tx.PreStep.IsEmpty() {
	//		return errors.New("source hash should  be empty")
	//	}
	//
	//	if to != params.SCAccount {
	//		return errors.New("invalid address:'to' should be system account")
	//	}
	//
	//	if header.MC != params.SCAccount {
	//		return ErrToMc
	//	}
	//case types.ActionContractDeal:
	//	if tx.PreStep.IsEmpty() {
	//		return errNotEmptySource
	//	}
	//
	//	originTx := tx.Tx
	//	if originTx == nil { //在已执行的Action中不断寻找
	//		originTx, err = dagchain.QueryOriginTx(tx)
	//		if err != nil {
	//			return err
	//		}
	//	}
	//	//防止出错
	//	if originTx == nil {
	//		return fmt.Errorf("not found the tranction info %s", tx.TxHash.Hex())
	//	}
	//	// 部署合约
	//	if sc.IsCallCreateContractTx(originTx.To(), originTx.Data()) {
	//		if params.SCAccount == header.MC { //2号单元：在系统链上给新合约分配地址和见证人
	//			//如果是部署合约则必须发送在系统链上
	//		} else { //3 号单元，
	//			//如果上一个单元中无交易信息，则必须是部署合约的4号单元（初始化合约）
	//			//否则是合约的第一个单元,(在合约链上部署合约)
	//			if header.Number != 1 {
	//				return consensus.ErrInvalidNumber
	//			}
	//			// 获取上一个位置的执行信息
	//			preTxEx, err := dagchain.dag.TxExecInfo(tx.PreStep.Hash, tx.TxHash)
	//			if err != nil {
	//				if err == ErrUnitNotFind { //单元不存在
	//					return consensus.ErrUnknownAncestor
	//				}
	//				return err
	//			}
	//			contractInfo, err := sc.GetContractOutput(sc.FuncCreateContract, preTxEx.Result.CallContractOutput)
	//			if err != nil {
	//				return err
	//			}
	//			//必须发送在合约链上
	//			info := contractInfo.(sc.NewCtWit)
	//			if info.Contract != header.MC {
	//				return ErrToMc
	//			}
	//		}
	//
	//	} else { //执行合约
	//		if originTx.To() != header.MC { //必须发生在合约链上
	//			return ErrToMc
	//		}
	//	}
	//
	//case types.ActionContractReceipt, types.ActionContractRefund:
	//	if tx.Tx != nil {
	//		return errNotNilTransaction
	//	}
	//
	//	if tx.PreStep.IsEmpty() {
	//		return errNotEmptySource
	//	}
	//	_, err := dagchain.dag.TxExecInfo(tx.PreStep.Hash, tx.TxHash)
	//	if err != nil {
	//		if err == ErrUnitNotFind { //单元不存在
	//			return consensus.ErrUnknownAncestor
	//		}
	//		return err
	//	}
	//	for _, cf := range tx.Result.Coinflows {
	//		// 如果coinflow转出方不是链地址，则报错
	//		if cf.To != header.MC {
	//			return ErrToMc
	//		}
	//	}
	//
	//}
	//
	return nil

	//todo 转账是否是判断同一个from 余额更新coinflow中to是否是同一个人 合约执行中与的原始单元与执行单元的to结果

	//txHash := tx.RealTxHash()
	//
	//var storedUnit *types.Unit
	//// 交易不允许重复
	//_, mc, unitHash, number, _ := GetTransaction(v.bc.chainDB, txHash)
	//// 如果存在，且所在单元不一致，则说明重复
	//if !unitHash.Empty() {
	//	//如果在同一高度，则必须保证：
	//	storedUnit = GetUnit(v.bc.chainDB, unitHash, mc, number)
	//} else {
	//	// 判断是否存在相同高度的单元，但是交易hash有不一样
	//	storedUnitHash := GetCanonicalHash(v.bc.chainDB, header.MC, header.Number)
	//	if !storedUnitHash.Empty() {
	//		storedUnit = GetUnit(v.bc.chainDB, storedUnitHash, header.MC, header.Number)
	//	}
	//}
	//// 如果相关单元存在，则必须保证:只允许同样交易的交易被同一批见证人多次各自签名发布
	//if storedUnit != nil {
	//	storedHeader := storedUnit.Header()
	//	// 1. 如果不在同一个高度，必然双花
	//	if storedHeader.MC != mc || storedHeader.Number != number {
	//		return ErrDoubleSpendTx
	//	}
	//	// 投票哈希相同，才能说明是同一批见证人
	//	// ysqi: 不能使用此判断，因为可能是由同一批见证人中的随机一批人所投票签名
	//	//if storedHeader.VoteHash != header.VoteHash {
	//	//	return ErrDoubleSpendTx
	//	//}
	//
	//	// 相同高度只允许一笔交易
	//	if storedHeader.TxsRoot != header.TxsRoot {
	//		return ErrDoubleSpendTx
	//	}
	//	// 相同高度只允许同一批次见证人
	//	if storedHeader.WitnessHash != header.WitnessHash {
	//		return ErrDoubleSpendTx
	//	}
	//	if storedHeader.ResultsRoot != header.ResultsRoot {
	//		return ErrDoubleSpendTx
	//	}
	//	if storedHeader.Timestamp != header.Timestamp {
	//		return ErrDoubleSpendTx
	//	}
	//	if storedHeader.TxsRoot != header.TxsRoot {
	//		return ErrDoubleSpendTx
	//	}
	//	if storedHeader.ReceiptRoot != header.ReceiptRoot {
	//		return ErrDoubleSpendTx
	//	}
	//	if storedHeader.StateRoot != header.StateRoot {
	//		return ErrDoubleSpendTx
	//	}
	//
	//	// 同一单元发布者不能重复发布
	//	sender1, err := storedUnit.Sender()
	//	if err != nil {
	//		return err
	//	}
	//	if sender1 == sender {
	//		return ErrDoubleSpendTx
	//	}
	//}
	//return nil
}

func (v *UnitValidator) SetTxSenderValidator(f func(tx *types.Transaction) error) {
	v.validWithCache = f
}

func (v *UnitValidator) ValidateTxSender(tx *types.Transaction) error {
	if v.validWithCache != nil {
		return v.validWithCache(tx)
	}
	_, err := types.Sender(v.signer, tx)
	return err
}

// ValidateBody 校验给定单元父单元、交易签名、共识签名是否正确
func (v *UnitValidator) ValidateBody(unit *types.Unit) error {
	tr := tracetime.New()
	defer tr.Stop()

	// 先与缓存对比
	tr.Tag()
	txs := unit.Transactions()

	//unit tx 不允许为空
	if len(txs) == 0 {
		return errors.New("unit's Transactions is empty")
	}
	tr.Tag()
	//黑球
	if unit.IsBlackUnit() {
		if len(unit.Transactions()) != 1 {
			return errors.New("black unit's tx length must equal 1")
		}
		if unit.Transactions()[0].Tx == nil {
			return errors.New("black unit's tx[0] can not be empty")
		}
	}

	//获取这个图
	scHeader := v.bc.GetHeaderByHash(unit.SCHash())
	if scHeader == nil {
		return consensus.ErrUnknownAncestor
	}
	sysdb, err := v.bc.StateAt(scHeader.MC, scHeader.StateRoot)
	if err != nil {
		return err
	}

	var (
		gasPriceLimit uint64
		gasLimit      = unit.GasLimit()
		limit         uint64
		unitTime      = time.Unix(0, int64(unit.Timestamp()))
	)
	getGasPrice := func() uint64 {
		if gasPriceLimit == 0 {
			gasPriceLimit, err = sc.ReadConfig(sysdb, params.ConfigParamsLowestGasPrice)
			if err != nil {
				panic(err)
			}
		}
		return gasPriceLimit
	}

	getPowDiff := func() uint64 {
		if limit == 0 {
			l, err := sc.ReadConfig(sysdb, params.ConfigParamsMinFreePoW)
			if err != nil {
				panic(err)
			}
			limit = l
		}
		return limit
	}

	getGasLimit := func() uint64 { return gasLimit }

	//根据单元的类型进行检验 是否该对应的类型 应有的对应关系
	for _, tx := range txs {
		if !tx.PreStep.IsEmpty() && tx.Tx != nil {
			return errors.New("invalid source or tx field")
		}

		if !tx.PreStep.IsEmpty() {
			stable := v.bc.GetHeader(tx.PreStep.Hash)
			if stable == nil {
				return consensus.ErrUnknownAncestor
			}
			if stable.MC != tx.PreStep.ChainID || stable.Number != tx.PreStep.Height {
				return errors.New("invalid pre step")
			}
			if stable.Timestamp >= unit.Timestamp() {
				return fmt.Errorf("timestamp %d must be greater than prestep's %d",
					unit.Timestamp(), stable.Timestamp)
			}
		}

		//对原始交易的合法性进行校验
		if tx.Tx != nil {
			if tx.TxHash != tx.Tx.Hash() {
				return errors.New("tx hash not matched")
			}

			//校验交易签名
			if err := v.ValidateTxSender(tx.Tx); err != nil {
				return err
			}

			txSender, err := types.Sender(v.signer, tx.Tx)
			if err != nil {
				return err
			}
			//不允许此地址发送交易
			if txSender == params.PublicPrivateKeyAddr && !unit.IsBlackUnit() {
				return ErrDisableSender
			}
			if _, err := VerifyTxBaseData(txSender, tx.Tx, unitTime, getGasPrice, getPowDiff, getGasLimit); err != nil {
				return err
			}
		}
	}

	//根据具体的action检查数据
	return nil
}

//
////
//func (v *UnitValidator) validateUnitByTxAction(mc common.Address, txExes types.TransactionExec) error {
//	switch txExes.Action {
//	case types.ActionTransferPayment: //转账
//		//判断所以的txs 数据的from是否一致 即都是mc
//		sender0, err := txExes.Tx.Sender()
//		if err != nil {
//			return err
//		}
//		if mc != sender0 {
//			return ErrToMc
//		}
//		return nil
//	case types.ActionTransferReceipt, types.ActionContractReceipt, types.ActionContractRefund: //收账  冻结返回 执行合约后合约中有转账给多个其他账户时的接收者收款
//		//判断所以的to 是否是一致 且要是mc
//		for _, coin := range txExes.Result.Coinflows {
//			if coin.To != mc {
//				return ErrToNotMc
//			}
//		}
//
//		return nil
//	case types.ActionContractDeal: //合约执行
//		//只检查source是否存在即可
//		header := v.bc.GetHeaderByHash(txExes.Source)
//		if header == nil {
//			return ErrNonRelationHeader
//		}
//		tx := v.bc.GetTransaction(txExes.Source)
//		if tx != nil {
//			return fmt.Errorf("unkonwn tx %v", txExes.Source)
//		}
//		// if tx.To() == nil { // new contact
//		if sc.IsCallCreateContractTx(tx.To(), tx.Data()) {
//			//TODO(ysqi)
//			// get the new contact address
//			// muse be match the mc
//		} else if tx.To() != mc {
//			return ErrToNotMc
//		}
//		return nil
//
//	case types.ActionContractFreeze: //冻结
//		//判断所以的txs 数据的from是否一致 即都是mc
//		sender0, err := txExes.Tx.Sender()
//		if err != nil {
//			return err
//		}
//		if mc != sender0 {
//			return ErrToMc
//		}
//		//todo 也可以检验合约地址是否存在
//		return nil
//	default:
//		return ErrNotMatchTxAction
//	}
//
//}

// ValidateState validates the various changes that happen after a state
// transition, such as amount of used gas, the receipt roots and the state root
// itself. ValidateState returns a database batch if the validation was a success
// otherwise nil and an error is returned.
func (v *UnitValidator) ValidateState(unit, _ *types.Unit, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error {
	var ok bool
	defer func() {
		if !ok {
			unitData, _ := json.MarshalIndent(unit, "", "\t")
			remote, _ := json.MarshalIndent(receipts, "", "\t")

			fmt.Fprintf(os.Stdout, "ValidateState failed:%s,from: %s \nunitInfo:\n%s\n\nreceipts:\n%s\n============\n",
				unit.Hash().Hex(), unit.Proposer().Hex(), string(unitData), string(remote))
		}
	}()

	// 验证状态
	if unit.GasUsed() != usedGas {
		return fmt.Errorf("invalid gas used (remote: %v local: %v)", unit.GasUsed(), usedGas)
	}
	// 验证状态Root 是否一致
	remoteRoot := unit.Root()
	if root, _ := statedb.IntermediateRoot(unit.MC()); remoteRoot != root {
		return fmt.Errorf("invalid state merkle root hash(remote: %x local: %x)", remoteRoot, root)
	}
	remoteRec := unit.ReceiptRoot()
	if got := types.DeriveSha(receipts); remoteRec != got {
		return fmt.Errorf("invalid receipt merkle root hash (remote: %x local: %x)", remoteRec, got)
	}
	// 验证 Bloom
	if rbloom := types.CreateBloom(receipts); rbloom != unit.Bloom() {
		return fmt.Errorf("invalid bloom (remote: %x  local: %x)", unit.Bloom(), rbloom)
	}
	ok = true
	return nil
}

// CalcGasLimit 根据父单元计算下一个单元的Gas限制.
// 该结果也许被调用者修改，是见证人见证时调整，非共识部分。
func CalcGasLimit(parent *types.Unit) uint64 {
	// contrib = (parentGasUsed * 3 / 2) / 1024
	contrib := (parent.GasUsed() + parent.GasUsed()/2) / params.GasLimitBoundDivisor

	// decay = parentGasLimit / 1024 -1
	decay := parent.GasLimit()/params.GasLimitBoundDivisor - 1

	/*
		strategy: gasLimit of block-to-mine is set based on parent's
		gasUsed value.  if parentGasUsed > parentGasLimit * (2/3) then we
		increase it, otherwise lower it (or leave it unchanged if it's right
		at that usage) the amount increased/decreased depends on how far away
		from parentGasLimit * (2/3) parentGasUsed is.
	*/
	limit := parent.GasLimit() - decay + contrib
	if limit < params.MinGasLimit {
		limit = params.MinGasLimit
	}
	// however, if we're now below the target (TargetGasLimit) we increase the
	// limit as much as we can (parentGasLimit / 1024 -1)
	if limit < params.TargetGasLimit {
		limit = parent.GasLimit() + decay
		if limit > params.TargetGasLimit {
			limit = params.TargetGasLimit
		}
	}
	return limit
}

// 验证交易的基本信息是否合法
func VerifyTxBaseData(sender common.Address, tx *types.Transaction, now time.Time,
	getMinGasPrice func() uint64,
	getPowDiff func() uint64,
	getGasLimit func() uint64) (isFree bool, err error) {
	if tx.Size() > 32*1024 {
		return false, newInvalidTx(ErrOversizedData)
	}

	if tx.Amount().Sign() < 0 {
		return false, newInvalidTx(ErrNegativeValue)
	}
	if sender != tx.SenderAddr() || sender.Empty() {
		return false, newInvalidTx(ErrInvalidSender)
	}

	if tx.To().Empty() {
		return false, newInvalidTx(ErrEmptyToAddress)
	}
	// 交易的过期时间必须在有效范围内
	if tx.Timeout() < types.DefTxMinTimeout || tx.Timeout() > types.DefTxMaxTimeout {
		return false, newInvalidTx(ErrInvalidTimeout)
	}
	// 不能过期
	if tx.Expired(now) {
		return false, newInvalidTx(ErrTxExpired)
	}

	//如果交易时间炒股
	if tx.IsFuture(now) {
		return false, newInvalidTx(errors.New("is future transaction"))
	}

	isFree = IsFreeTx(tx)

	if isFree {
		// 免费交易只会出现在系统链上
		if tx.To() != params.SCAccount {
			return false, newInvalidTx(ErrInvalidSeed)
		}
		// 免费交易gas必须为0
		if tx.GasPrice() > 0 || tx.Gas() > 0 {
			return false, newInvalidTx(ErrInvalidGas)
		}
		if tx.Amount().Sign() != 0 {
			return false, newInvalidTx(errors.New("invalid tx value,should be zero"))
		}

	} else {
		if tx.Gas() == 0 || tx.Gas() > getGasLimit() {
			return false, newInvalidTx(ErrInvalidGas)
		}

		if tx.GasPrice() < getMinGasPrice() {
			return false, newInvalidTx(ErrGasPriceTooLow)
		}

		if tx.Seed() > 0 {
			return false, newInvalidTx(ErrInvalidSeed)
		}

		//不得溢出
		if _, overflow := tx.GasCost(); overflow {
			return false, newInvalidTx(errors.New("gas*price is overflow uint64"))
		}
	}

	//如果是发送到系统链，则只有在一些操作才允许
	powFunc := bytes.HasPrefix(tx.Data(), sc.FuncApplyWitnessIDBytes) ||
		bytes.HasPrefix(tx.Data(), sc.FuncReplaceWitnessIDBytes)

	if powFunc {
		//如果不是发生给系统链，错误
		if tx.To() != params.SCAccount {
			return false, newInvalidTx(ErrInvalidSeed)
		}
		if tx.Seed() == 0 {
			return false, newInvalidTx(ErrInvalidSeed)
		}
		//此时校验 Seed
		if ethash.VerifyProofWork(tx, new(big.Int).SetUint64(getPowDiff())) == false {
			return false, newInvalidTx(ErrInvalidSeed)
		}

	} else if tx.Seed() != 0 {
		return false, newInvalidTx(ErrInvalidSeed)
	}

	return isFree, nil
}
