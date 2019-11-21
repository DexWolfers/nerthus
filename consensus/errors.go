package consensus

import (
	"errors"
	"fmt"

	"gitee.com/nerthus/nerthus/core/types"
)

var (
	// ErrFutureBlock 单元时间戳超过当前节点当下时间
	ErrFutureBlock = errors.New("unit in the future")

	ErrInsertingBlock = errors.New("unit in inserting")

	// ErrInvalidNumber 单元编号未等于父单元编号➕1
	ErrInvalidNumber = errors.New("invalid unit number")

	// ErrInvalidHashField 非法的哈希字段
	ErrInvalidHashField = errors.New("invalid the hash field of unit")

	// ErrInvalidMCAccount 单元所在账户链和父单元不一致
	ErrInvalidMCAccount = errors.New("invalid unit main chain account")

	// ErrKnownBlock is returned when a unit to import is already known locally.
	ErrKnownBlock = errors.New("unit already known")

	ErrInvalidUnitSender = errors.New("the unit sender is not witness")

	ErrNotEnoughWitnessVotes = errors.New("not enough witness's votes")

	ErrInvalidWitness = errors.New("invalid witness work")

	// ErrInvalidSCHash 非法SC Hash
	ErrInvalidSCHash = errors.New("invalid unit sc hash ")

	// ErrInvalidProposer
	ErrInvalidProposer = errors.New("invalid proposer")

	// ErrInvalidSCNumber
	ErrInvalidSCNumber = errors.New("invalid system chain number")

	// ErrNotFoundHeader 缺失头信息
	ErrNotFoundHeader = errors.New("not found unit header")

	// ErrAlreadyExistStableUnit 已在相同高度存在稳定单元
	ErrAlreadyExistStableUnit = errors.New("stable units already exist at the same height")

	// ErrCreateMulUnits 同一个见证人在相同高度生成多个单元
	ErrCreateMulUnits = errors.New("create multiple units at the same height")
	// ErrMultipleLawfulUnits 出现重复的合法单元
	ErrMultipleLawfulUnits = errors.New("appeared multiple lawful units at the same height")

	ErrSeal            = errors.New("vote has expired")                        //投票离出单元的时间太长
	ErrAlreadyHadVoted = errors.New("already had the vote in the same height") //同一高度已有投票了
	// ErrRepeatingVoting 对同一单元多次投票
	ErrRepeatingVoting = errors.New("repeating voting for same unit")

	ErrUnknownAncestor = errors.New("miss ancestor")

	ErrLessVotes = errors.New("votes too little")

	ErrInInArbitration = errors.New("chain is in arbitration")
)

// ErrWitnessesDishonest 见证人不诚实行为错误
type ErrWitnessesDishonest types.FoundBadWitnessEvent

func (err *ErrWitnessesDishonest) Error() string {
	return fmt.Sprintf("witness are dishonest: %v", err.Reason)
}

// 失败错误信息是否是写入单元时的错误信息
func IsBadUnitErr(err error) bool {
	switch err {
	default:
		return true
	case ErrKnownBlock,
		ErrFutureBlock,
		ErrInsertingBlock,
		ErrInInArbitration,
		ErrAlreadyExistStableUnit,
		ErrUnknownAncestor:
		return false
	}
}
