package backend

import (
	"errors"
	"fmt"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

const (
	defGasCap = uint64(0x7fffffffffffffff) - 1 //[params.MinGasLimit,2^63-1]

	//用户链必须引用30000个系统链单元内的单元，相当于31天。单位纳秒
	maxTimeDiff = uint64(time.Duration(params.OneNumberHaveSecondBySys*30000) * time.Second)
)

var (
	// errInvalidProposal is returned when a prposal is malformed.
	errInvalidProposal = errors.New("invalid proposal")
	// errInvalidSignature is returned when given signature is not signed by given
	// address.
	errInvalidSignature = errors.New("invalid signature")
	// errUnknownBlock is returned when the list of validators is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")
	// errUnauthorized is returned if a header is signed by a non authorized entity.
	errUnauthorized = errors.New("unauthorized")
	// errInvalidDifficulty is returned if the difficulty of a block is not 1
	errInvalidDifficulty = errors.New("invalid difficulty")
	// errInvalidExtraDataFormat is returned when the extra data format is incorrect
	errInvalidExtraDataFormat = errors.New("invalid extra data format")
	// errInvalidMixDigest is returned if a block's mix digest is not Istanbul digest.
	errInvalidMixDigest = errors.New("invalid Istanbul mix digest")
	// errInvalidNonce is returned if a block's nonce is invalid
	errInvalidNonce = errors.New("invalid nonce")
	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")
	// errInvalidTimestamp is returned if the timestamp of a block is lower than the previous block's timestamp + the minimum block period.
	errInvalidTimestamp = errors.New("invalid timestamp")
	// errInvalidCommittedSeals is returned if the committed seal is not signed by any of parent validators.
	errInvalidCommittedSeals = errors.New("invalid committed seals")
	// errEmptyCommittedSeals is returned if the field of committed seals is zero.
	errEmptyCommittedSeals = errors.New("zero committed seals")
)
var (
	now                                  = time.Now
	allowedFutureBlockTime time.Duration = 15 * time.Second
)

// Start engine
func (sb *backend) Start() error {

	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return bft.ErrStartedEngine
	}
	sb.core.start()
	sb.coreStarted = true
	return nil
}

// Stop engine
func (sb *backend) Stop() error {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return bft.ErrStoppedEngine
	}
	sb.core.stop()
	sb.coreStarted = false
	return nil
}

// Validators returns the validator set
func (sb *backend) Validators(mc common.Address, header *types.Header) (bft.ValidatorSet, error) {
	scHeader := sb.chain.GetChainTailHead(params.SCAccount)
	if scHeader == nil || scHeader.Hash().Empty() {
		return nil, errors.New("get tail header failed")
	}
	isGet := true
	// header==nil 强行刷新
	if header == nil {
		header = sb.chain.GetChainTailHead(mc)
		isGet = false
	}
	// 共用一份见证人缓存
	var vset bft.Validators
	sb.core.validators.RLock()
	//log.Debug(">>>>>>>>get validator", "mc", mc, "number", header.Number, "cache_number", sb.core.validators.height, "cached", len(sb.core.validators.validator))
	if isGet && header.Number != 0 && header.Number == sb.core.validators.height {
		vset = sb.core.validators.validator
		sb.core.validators.RUnlock()
	} else {
		sb.core.validators.RUnlock()
		witnessList, err := sb.chain.GetChainWitnessLib(mc, scHeader.Hash())
		if err != nil {
			return nil, err
		}
		for i := range witnessList {
			vset = append(vset, bft.NewDefValidator(witnessList[i]))
		}
		// 缓存最新number
		sb.core.validators.Lock()
		if header.Number > sb.core.validators.height {
			sb.core.validators.height = header.Number
			sb.core.validators.validator = vset
		}
		sb.core.validators.Unlock()
	}
	return bft.NewSet(header.Number+1, header.Hash(), vset, mc), nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
// 交易单元头
func (sb *backend) VerifyHeader(chain consensus.ChainReader, unit *types.Unit, seal bool) error {
	return sb.verifyHeader(chain, unit, seal)
}

// 与已校验提案对比，提高性能
func (sb *backend) VerifyWithProposal(unit *types.Unit) (*bft.MiningResult, bool) {
	eng := sb.core.getEngine(unit.MC())
	if eng == nil {
		return nil, false
	}
	pro := eng.CurrentProposal()
	if pro == nil {
		return nil, false
	}
	if pro.Unit.Hash() != unit.Hash() {
		return nil, false
	}
	// 校验投票
	chainCurrentWitness, err := sb.chain.GetChainWitnessLib(unit.MC(), unit.SCHash())
	if err != nil {
		return nil, false
	}
	if err := sb.VerifySeal(sb.chain, unit, chainCurrentWitness); err != nil {
		return nil, false
	}
	return pro, true
}

// get unit proposer
// 获取单元提案人
func (sb *backend) Author(unit *types.Unit) (common.Address, error) {
	sb.logger.Trace("get author", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash())
	// 验证单元签名
	return types.Sender(sb.sign, unit)
}

// verify unit
// 校验单元
func (sb *backend) VerifySeal(chain consensus.ChainReader, unit *types.Unit, chainWitness []common.Address) error {
	defer tracetime.New().SetMin(time.Second).Stop()

	//如果链此时无见证人
	if len(chainWitness) == 0 {
		return consensus.ErrInvalidUnitSender
	}
	// 快速过滤无效单元，防止包含大量无效签名进行CPU攻击
	if len(unit.WitnessVotes()) < params.GetChainMinVoting(unit.MC()) {
		return consensus.ErrNotEnoughWitnessVotes
	} else if len(unit.WitnessVotes()) > params.GetChainWitnessCount(unit.MC()) {
		return consensus.ErrNotEnoughWitnessVotes
	}

	// 验证单元签名
	general, err := types.Sender(sb.sign, unit)
	if err != nil {
		return err
	}

	// 将军必须是见证人
	list := common.AddressList(chainWitness)
	if !list.Have(general) {
		return consensus.ErrInvalidUnitSender
	}

	// 投票检查
	witnessList, err := unit.Witnesses()
	if err != nil {
		return err
	}
	//检查unit中见证人commit投票是否充足/太多
	if len(witnessList) < params.GetChainMinVoting(unit.MC()) {
		return consensus.ErrNotEnoughWitnessVotes
	} else if len(witnessList) > params.GetChainWitnessCount(unit.MC()) {
		return consensus.ErrNotEnoughWitnessVotes
	}
	for _, w := range witnessList {
		if !list.Have(w) {
			sb.logger.Debug("witnesslist", "leader", general.String(), "w", w.String(),
				"list", common.AddressList(witnessList).String(), "curList", common.AddressList(chainWitness).String())
			return consensus.ErrInvalidWitness
		}
	}

	return sb.dishonestCheck(unit)
}

// 校验理事仲裁单元
func (sb *backend) verifyArbitrationUnit(chain consensus.ChainReader, unit *types.Unit) error {
	err := sb.verifyHeader(chain, unit, false)
	if err != nil {
		return err
	}
	// 时间戳间隔校验
	lastUnit := chain.GetHeaderByNumber(unit.MC(), unit.SCNumber())
	rightTime := lastUnit.Timestamp + uint64((time.Duration(params.SCDeadArbitrationInterval) * time.Second).Nanoseconds())
	if rightTime != unit.Timestamp() {
		return errors.New("invalid unit timestamp")
	}
	// 投票校验 投票人必须全部是理事
	votes, err := unit.Witnesses()
	if err != nil {
		return err
	}
	statedb, err := chain.GetChainTailState(params.SCAccount)
	if err != nil {
		return err
	}
	coucils, err := sc.GetCouncilList(statedb)
	if err != nil {
		return err
	}
	// 票数必须>2/3
	if len(votes) < len(coucils)*2/3+1 {
		return errors.New("commit votes not enough")
	}
	for _, v := range votes {
		is := false
		for _, c := range coucils {
			if v == c {
				is = true
				break
			}
		}
		if !is {
			return errors.New("invalid votes,must voted by council")
		}
	}

	// 如果已收到链尾单元与特殊单元冲突，删除链尾单元
	u := chain.GetChainTailHead(unit.MC())
	if u.Number > unit.Number() {
		return errors.New("invalid arbitration unit:at history number")
	}

	return nil
}

// 作恶检查
func (sb *backend) dishonestCheck(unit *types.Unit) error {
	//检查是否存在重复高度单元
	h := sb.chain.GetStableHash(unit.MC(), unit.Number())
	if h.Empty() || h == unit.Hash() {
		return nil
	}
	//在已稳定高度广播另一个合法单元，属集体作恶
	oldHeader := sb.chain.GetHeaderByHash(h)
	if oldHeader == nil {
		return errors.New("not found header by hash")
	}
	votes, err := sb.chain.GetVoteMsg(h)
	if err != nil {
		return err
	}
	oldVotes := make([]types.WitenssVote, len(votes))
	for i, v := range votes {
		oldVotes[i] = v.ToWitnessVote()
	}
	oldStateDB, err := sb.chain.GetUnitState(oldHeader.SCHash)
	if err != nil {
		return err
	}
	oldChainNumber := sc.GetChainWitnessStartNumber(oldStateDB, unit.MC())
	newStateDB, err := sb.chain.GetUnitState(unit.SCHash())
	if err != nil {
		return err
	}
	// 高度不一样，最终选择合法单元方式不一样
	newChainNumber := sc.GetChainWitnessStartNumber(newStateDB, unit.MC())
	if newChainNumber > oldChainNumber && newChainNumber >= unit.Number() {
		return &consensus.ErrWitnessesDishonest{
			Reason:    consensus.ErrMultipleLawfulUnits,
			Witness:   oldHeader.Proposer,
			Bad:       *oldHeader,
			BadVotes:  oldVotes,
			Good:      *unit.Header(),
			GoodVotes: unit.WitnessVotes(),
		}
	}
	general, _ := types.Sender(sb.sign, unit)
	//收集2个单元的所有参与者
	return &consensus.ErrWitnessesDishonest{
		Reason:    consensus.ErrMultipleLawfulUnits,
		Witness:   general,
		Bad:       *unit.Header(),
		BadVotes:  unit.WitnessVotes(),
		Good:      *oldHeader,
		GoodVotes: oldVotes,
	}
}

// verify header
func (sb *backend) verifyHeader(chain consensus.ChainReader, unit *types.Unit, seal bool) error {
	tr := tracetime.New("chain", unit.MC(), "number", unit.Number()).SetMin(0)
	defer tr.Stop()

	if chain.GetHeader(unit.Hash()) != nil {
		return consensus.ErrKnownBlock
	}
	tr.Tag()
	parent := chain.GetHeaderByHash(unit.ParentHash())
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	tr.Tag()
	header := unit.Header()
	if header.Number != 1 && header.MC != parent.MC {
		return consensus.ErrInvalidMCAccount
	}
	// Verify that the unit number is parent's +1
	if header.Number != parent.Number+1 {
		return consensus.ErrInvalidNumber
	}

	if header.Timestamp > uint64(time.Now().Add(allowedFutureBlockTime).UnixNano()) {
		return consensus.ErrFutureBlock
	}
	tr.Tag()
	errNumber := fmt.Errorf("header scNumber %d not equal parent number %d", header.SCNumber, parent.Number)
	// 单元时间戳在父单元之前
	if header.Timestamp <= parent.Timestamp {
		return fmt.Errorf("timestamp %d equals parent's %d", header.Timestamp, parent.Timestamp)
	}
	minDiff := params.OneNumberHaveSecondByUc
	if params.SCAccount == header.MC {
		minDiff = params.OneNumberHaveSecondBySys
	}

	// 必须保证单元间隔
	if header.Timestamp-parent.Timestamp < uint64(minDiff)*uint64(time.Second) {
		return errors.New("unit timestamp interval not enough")
	}
	tr.Tag()
	// 判定提案者
	signer := types.NewSigner(chain.Config().ChainId)
	proposer, err := types.Sender(signer, unit)
	if err != nil {
		return err
	}
	tr.Tag()
	if header.Proposer != proposer {
		return consensus.ErrInvalidProposer
	}

	tr.Tag()

	//将根据此获取系统链配置
	var scParent *types.Header
	if header.MC == params.SCAccount {
		if header.SCHash != header.ParentHash {
			return consensus.ErrInvalidSCHash
		}
		if header.SCNumber != parent.Number {
			return errNumber
		}
		scParent = parent
	} else {
		tr.Tag()
		scParent = chain.GetHeaderByHash(header.SCHash)
		if scParent == nil {
			return consensus.ErrUnknownAncestor
		}
		tr.Tag()
		if header.SCNumber != scParent.Number {
			return fmt.Errorf("header scNumber %d not equal scParent number %d", header.SCNumber, scParent.Number)
		}
		//时间不能早于系统链时间
		if header.Timestamp <= scParent.Timestamp {
			return fmt.Errorf("timestamp %d equals system chain parent's %d", header.Timestamp, scParent.Timestamp)
		}
		//用户链单元不允许引用非常老旧的系统链单元，31天前的系统链单元
		if diff := header.Timestamp - scParent.Timestamp; diff > maxTimeDiff {
			return errors.New("use too long system chain unit")
		}
	}
	// 引用系统链时必须保证其引用不会交叉：
	if header.SCNumber < parent.SCNumber {
		return fmt.Errorf("invalid system number: have %d,want >=%d", header.SCNumber, parent.SCNumber)
	}
	tr.Tag()

	// 验证 gasLimit 合理区间为 [min(5000), 2^63-1]
	if header.GasLimit > defGasCap {
		return fmt.Errorf("invalid gasLimit: have %d, max %d", header.GasLimit, defGasCap)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}
	//不能低于最低标准，等价于至少包含一笔交易
	//if header.GasUsed < params.MinGasLimit {
	//	return fmt.Errorf("invalid gasUsed: have %d, want >= %d", header.GasUsed, params.MinGasLimit)
	//}
	tr.Tag()

	if limit, err := sc.ReadConfigFromUnit(chain, scParent, params.ConfigParamsUcGasLimit); err != nil {
		return err
	} else if header.GasLimit > limit {
		return fmt.Errorf("invalid gasLimit: have %d, want <= %d", header.GasLimit, limit)
	}
	tr.Tag()
	// Verify the engine specific seal securing the unit
	if seal {
		// 特殊单元处理
		if unit.IsBlackUnit() {
			return sb.verifyArbitrationUnit(chain, unit)
		}

		// 见证人必须出现在用户见证人列表中
		chainCurrentWitness, err := chain.GetChainWitnessLib(unit.MC(), unit.SCHash())
		if err != nil {
			if err == types.ErrNotFoundHeader { //也许系统链单元还尚未落地，因此按缺失父单元处理
				return consensus.ErrUnknownAncestor
			}
			return err
		}
		if err := sb.VerifySeal(chain, unit, chainCurrentWitness); err != nil {
			return err
		}

	}
	return nil
}
