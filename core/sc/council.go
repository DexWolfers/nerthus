package sc

import (
	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

//go:generate enumer -type=CouncilStatus -json -transform=snake -trimprefix=CouncilStatus
type CouncilStatus uint8

const (
	CouncilStatusValid   CouncilStatus = iota + 1 // 有效
	CouncilStatusInvalid                          // 注销
)

// Council 理事成员
type Council struct {
	Address     common.Address // 地址
	Status      CouncilStatus  // 状态
	Margin      uint64         // 保证金
	ApplyHeight uint64
}

// CouncilList 理事成员列表
type CouncilList []Council

// 理事信息存储
func getCouncilInfoItem(c common.Address) SubItems {
	return SubItems{[]interface{}{PrefixCouncilInfo, c}}
}

// getCouncilItem 获取初始化的理事DB key
func getCouncilItem() StateSet {
	return StateSet([]interface{}{PrefixMemberTag})
}

// GetCouncilCount 获取理事总数
func GetCouncilCount(db StateDB) uint64 {
	return getCouncilItem().Len(db)
}

// GetCouncilAddr 根据索引获取理事
func GetCouncilAddr(db StateDB, flag uint64) (common.Address, error) {
	hash := getCouncilItem().Get(db, flag)
	if hash.Empty() {
		return common.Address{}, ErrNotCouncil
	}
	return hash.Address(), nil
}

// AddCouncil 添加理事
func AddCouncil(db StateDB, council *Council, ceiling uint64) error {
	// 写入理事详情
	WriteCouncilInfo(db, council)
	return nil
}

// RemoveCouncil 注销理事
func RemoveCouncil(db StateDB, address common.Address, sysNumber uint64) error {
	count := GetCouncilCount(db)
	if count == 1 {
		return errors.New("only one council, can't remove")
	}

	if s, ok := GetCouncilStatus(db, address); !ok {
		return errors.New("is not council")
	} else if s == CouncilStatusInvalid {
		// 修改理事详情的状态
		//状态已修改，不重复处理。因为可能是在公众投票踢出理事时，理事已自行退出
		return nil
	}

	item := getCouncilInfoItem(address)
	item.SaveSub(db, "status", CouncilStatusInvalid)

	list := getCouncilItem()
	if index, ok := list.Exist(db, address); ok {
		getCouncilItem().Remove(db, index)
	}

	p := GetSettlePeriod(sysNumber)
	PeriodInfo{db}.AddCouncilCount(p, -1) //当前周期的理事人数加-1
	return nil
}

// WriteCouncilInfo 写入理事详情
func WriteCouncilInfo(db StateDB, council *Council) {
	item := getCouncilInfoItem(council.Address)
	item.SaveSub(db, "status", council.Status)
	item.SaveSub(db, "margin", council.Margin)
	item.SaveSub(db, "applyHeight", council.ApplyHeight)

	// 写入地址指向的索引值
	getCouncilItem().Add(db, council.Address)

	PeriodInfo{db}.AddCouncilCount(GetSettlePeriod(council.ApplyHeight), 1) //当前周期的理事人数加1

	//初始化心跳记录
	initHearbeatLastRound(db, types.AccCouncil, council.Address, council.ApplyHeight)
}

// GetCouncilInfo 获取理事详情
func GetCouncilInfo(db StateDB, address common.Address) (*Council, error) {
	item := getCouncilInfoItem(address)
	status := item.GetSub(db, "status")
	if status.Empty() {
		return nil, ErrNotCouncil
	}
	var council Council
	council.Address = address
	council.Status = CouncilStatus(status.Big().Int64())
	council.Margin = item.GetSub(db, "margin").Big().Uint64()
	council.ApplyHeight = item.GetSub(db, "applyHeight").Big().Uint64()
	return &council, nil
}

type CouncilInfoLazy struct {
	db   StateDB
	item SubItems
	addr common.Address
}

func CreateCouncilInfoLazy(statedb StateDB, council common.Address) CouncilInfoLazy {
	item := getCouncilInfoItem(council)
	return CouncilInfoLazy{
		db:   statedb,
		item: item,
		addr: council,
	}
}
func (w CouncilInfoLazy) Address() common.Address {
	return w.addr
}
func (w CouncilInfoLazy) Status() (CouncilStatus, bool) {
	return GetCouncilStatus(w.db, w.addr)
}

func (w CouncilInfoLazy) Margin() uint64 {
	return w.item.GetSub(w.db, "margin").Big().Uint64()
}
func (w CouncilInfoLazy) ApplyHeight() uint64 {
	return w.item.GetSub(w.db, "apply_height").Big().Uint64()
}

// 存在理事信息
func ExistCounci(db StateDB, address common.Address) bool {
	return getCouncilInfoItem(address).GetSub(db, "status").Empty() == false
}

// IsCouncil 判断账户是否是理事
func IsActiveCouncil(db StateDB, address common.Address) bool {
	status, is := GetCouncilStatus(db, address)
	if !is {
		return false
	}
	return status == CouncilStatusValid
}

func GetCouncilStatus(db StateDB, address common.Address) (status CouncilStatus, isCouncil bool) {
	hash := getCouncilInfoItem(address).GetSub(db, "status")
	if hash.Empty() {
		return 0, false
	}
	return CouncilStatus(hash.Big().Int64()), true
}

// 获取理事保证金
func GetCounciMargin(db StateDB, address common.Address) uint64 {
	hash := getCouncilInfoItem(address).GetSub(db, "margin")
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}

// GetCouncilList 获取理事列表
func GetCouncilList(db StateDB) (common.AddressList, error) {
	var addrList common.AddressList
	getCouncilItem().Range(db, func(index uint64, value common.Hash) bool {
		addrList = append(addrList, value.Address())
		return true
	})
	return addrList, nil
}

// 遍历所有有效理事
func RangeCouncil(db StateDB, cb func(addr common.Address) bool) {
	getCouncilItem().Range(db, func(index uint64, value common.Hash) bool {
		return cb(value.Address())
	})
}
