package rawdb

import (
	"bytes"

	"gitee.com/nerthus/nerthus/log"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrNotFound = errors.Errorf("rawdb: not found by key")
)

type UnitRaw struct {
	Header  []byte //头数据
	Sign    []byte //单元将军签名
	Votes   []byte //签名
	TxSteps []byte //单元交易信息
	TxsInfo []byte // 交易具体信息
}

type unitLocationEntry struct {
	MC     common.Address
	Number uint64
}

// WriteHeader 序列化存储单元头到db.
// 同时存储hash和number关系。
func WriteHeader(db ntsdb.Putter, header *types.Header) {
	data, err := rlp.EncodeToBytes(header)
	if err != nil {
		gameover(err, "failed to encode Header")
	}
	loc := unitLocationEntry{
		MC:     header.MC,
		Number: header.Number,
	}
	dataloc, err := rlp.EncodeToBytes(loc)
	if err != nil {
		gameover(err, "failed to encode unitLocationEntry")
	}

	hash := header.Hash()

	//必须在记录单元时登记
	WriteTimeline(db, hash, header.Timestamp)

	// 位置信息
	key := append(unitHashPrefix, hash.Bytes()...)
	if err := db.Put(key, dataloc); err != nil {
		gameover(err, "failed to storage hash to number mapping")
	}

	//存储
	key = append(headerPrefix, hash.Bytes()...)
	if err := db.Put(key, data); err != nil {
		gameover(err, "failed to storage unit header to number mapping")
	}
}

// GetHeaderRLP 获取单元头RLP编码信息,
// 如果未找到，则为空
func GetHeaderRLP(db ntsdb.Getter, hash common.Hash) rlp.RawValue {
	return Get(db, headerKey(hash))
}

func GetHeader(db ntsdb.Getter, hash common.Hash) (*types.Header, error) {
	b := GetHeaderRLP(db, hash)
	if len(b) == 0 {
		return nil, ErrNotFound
	}
	return DecodeHeaderRLP(b)
}

func DecodeHeaderRLP(data []byte) (*types.Header, error) {
	header := new(types.Header)
	err := rlp.DecodeBytes(data, header)
	if err != nil {
		return nil, err
	}
	return header, nil
}
func decodeHeader(s *rlp.Stream) (*types.Header, error) {
	header := new(types.Header)
	err := s.Decode(header)
	if err != nil {
		return nil, err
	}
	return header, nil
}

func GetUnitSignRLP(db ntsdb.Database, hash common.Hash, number uint64) rlp.RawValue {
	return Get(db, unitSignKey(hash))
}
func DecodeUnitSign(b []byte) (*types.SignContent, error) {
	sign := new(types.SignContent)
	err := rlp.DecodeBytes(b, sign)
	if err != nil {
		return nil, err
	}
	return sign, nil
}

type TxStepExecStorage struct {
	HaveTx  bool
	TxHash  common.Hash
	Action  types.TxAction
	PreStep types.UnitID

	Tx *types.Transaction `rlp:"-"` //仅用于途中记录交易
}

func GetUnitTxStepExecRLP(db ntsdb.Getter, hash common.Hash, number uint64) rlp.RawValue {
	return Get(db, unitTxStepExecKey(hash, number))
}

func DecodeUnitTxStepExec(b []byte) ([]*TxStepExecStorage, error) {
	var list []*TxStepExecStorage
	if err := rlp.DecodeBytes(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func GetUnitVoteSignsRLP(db ntsdb.Getter, hash common.Hash, number uint64) rlp.RawValue {
	return Get(db, unitVoteSignsKey(hash, number))
}

func DecodeUnitVoteSigns(b []byte) ([]types.WitenssVote, error) {
	var list []types.WitenssVote
	if err := rlp.DecodeBytes(b, &list); err != nil {
		return nil, err
	}
	return list, nil
}

type UnitBodyStorage struct {
	Txs    []TxStepExecStorage //交易信息
	Voters []types.WitenssVote //投票签名
	Sign   []byte              //单元签名
}

func GetUnitBodyStorageRLP(db ntsdb.Getter, chain common.Address, uhash common.Hash, number uint64) rlp.RawValue {
	return Get(db, unitBodyKey(chain, uhash, number))
}
func GetUnitBodyStorage(db ntsdb.Getter, chain common.Address, uhash common.Hash, number uint64) *UnitBodyStorage {
	b := GetUnitBodyStorageRLP(db, chain, uhash, number)
	if len(b) == 0 {
		return nil
	}
	info, err := DecodeUnitBodyStorage(b)
	if err != nil {
		gameover(err, "failed to decode")
	}
	return info
}

func DecodeUnitBodyStorage(b []byte) (*UnitBodyStorage, error) {
	info := new(UnitBodyStorage)
	err := rlp.DecodeBytes(b, &info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
func decodeUnitBodyStorage(s *rlp.Stream) (*UnitBodyStorage, error) {
	info := new(UnitBodyStorage)
	err := s.Decode(&info)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// 写入单元Body
// Body包括：单元签名、单元交易和单元见证人投票信息
func WriteUnitBody(db ntsdb.Putter, chain common.Address, uhash common.Hash, number uint64, body *types.Body) {
	storage := UnitBodyStorage{
		Sign:   body.Sign.Get(),
		Voters: body.Votes,
	}
	oringTxs := make([]common.Hash, 0, len(body.Txs))
	storage.Txs = make([]TxStepExecStorage, len(body.Txs))
	for i, v := range body.Txs {
		storage.Txs[i] = TxStepExecStorage{
			TxHash:  v.TxHash,
			HaveTx:  v.Tx != nil,
			Action:  v.Action,
			PreStep: v.PreStep,
		}
		if v.Tx != nil {
			WriteTransaction(db, v.Tx)
			//需要去重吗？
			oringTxs = append(oringTxs, v.TxHash)
		}
	}
	// Body
	b, err := rlp.EncodeToBytes(storage)
	if err != nil {
		panic(err)
	}
	if err := db.Put(unitBodyKey(chain, uhash, number), b); err != nil {
		gameover(err, "failed to storage unit body")
	}
	// 同时记录存在交易原始信息的交易记录
	WriteUnitOriginTxs(db, uhash, number, oringTxs)
}

func DelUnitBody(db ntsdb.Database, chain common.Address, number uint64, uhash common.Hash) {
	db.Delete(unitBodyKey(chain, uhash, number))
	for _, tx := range GetUnitOringTxHash(db, uhash, number) {
		db.Delete(transactionKey(tx))
	}
	db.Delete(unitOriginTxsKey(uhash, number))
}
func ExistUnitBody(db ntsdb.Getter, chain common.Address, uhash common.Hash, number uint64) bool {
	return len(GetUnitBodyStorageRLP(db, chain, uhash, number)) > 0
}
func GetUnitBody(db ntsdb.Getter, chain common.Address, uhash common.Hash, number uint64) *types.Body {
	info := GetUnitBodyStorage(db, chain, uhash, number)
	txs, err := GetUnitOriginTxs(db, chain, uhash, number)
	if err != nil {
		log.Warn("failed to get txs", "chain", chain, "uhash", uhash, "number", number, "err", err)
		return nil
	}
	body, err := decodeUnitBody(info, txs)
	if err != nil {
		log.Warn("failed to decode", "chain", chain, "uhash", uhash, "number", number, "err", err)
		return nil
	}
	return body
}

func GetUnitOringTxHashRLP(db ntsdb.Getter, hash common.Hash, number uint64) rlp.RawValue {
	return Get(db, unitOriginTxsKey(hash, number))
}

func GetUnitOriginTxRLP(db ntsdb.Getter, hash common.Hash, number uint64) rlp.RawValue {
	txs := GetUnitOringTxHash(db, hash, number)
	return GetTxListRLP(db, txs)
}

func GetUnitOringTxHash(db ntsdb.Getter, hash common.Hash, number uint64) []common.Hash {
	b := GetUnitOringTxHashRLP(db, hash, number)
	if len(b) == 0 {
		return nil
	}
	var list []common.Hash
	if err := rlp.DecodeBytes(b, &list); err != nil {
		gameover(err, "failed rlp decode []common.Hash")
	}
	return list
}

// 单元的 Body由多个部分组成
//
// --------------------------------------------------------------------------------------
// |  rlp([]TxStepExecStorage+[]VoterSigns+UnitSign)  | rlp([]originTxs) |
// --------------------------------------------------------------------------------------
//
// 因此，在解码时需要使用正确方式解析，可以使用rlp.Stream 依次解析。
//
func GetUnitBodyRLP(db ntsdb.Database, chain common.Address, uhash common.Hash, number uint64) rlp.RawValue {
	return append(
		GetUnitBodyStorageRLP(db, chain, uhash, number),
		GetUnitOriginTxRLP(db, uhash, number)...)
}

func DecodeUnitBody(stream *rlp.Stream) (*types.Body, error) {

	body, err := decodeUnitBodyStorage(stream)
	if err != nil {
		return nil, err
	}

	originTxs, err := decodeTxs(stream)
	if err != nil {
		return nil, err
	}
	return decodeUnitBody(body, originTxs)
}

func decodeUnitBody(body *UnitBodyStorage, originTxs types.Transactions) (*types.Body, error) {

	//检查
	if len(originTxs) > len(body.Txs) {
		return nil, errors.Errorf("invalid origin tx list length")
	}
	for i, v := range body.Txs {
		if !v.PreStep.IsEmpty() && v.HaveTx {
			return nil, errors.New("invalid transaction exec data")
		}
		if v.HaveTx {
			for j, tx := range originTxs {
				if tx.Hash() == v.TxHash {
					body.Txs[i].Tx = originTxs[j]
					break
				}
			}
			if body.Txs[i].Tx == nil {
				return nil, errors.Errorf("invalid origin tx list,missing tx %s", v.TxHash.Hex())
			}
		}
	}

	txs := make(types.TxExecs, len(body.Txs))
	for i, v := range body.Txs {
		txs[i] = &types.TransactionExec{
			Action:  v.Action,
			PreStep: v.PreStep,
			TxHash:  v.TxHash,
			Tx:      v.Tx,
		}
	}

	sign := (&types.SignContent{}).Set(body.Sign)
	//注意：此处不进行单元数据合法性校验
	return &types.Body{
		Txs:   txs,
		Sign:  *sign,
		Votes: body.Voters,
	}, nil
}

// 单元的 Body由多个部分组成
//
// -----------------------------------------
// | rlp(Header) | rlp(UnitBodyStorage)  |
// -----------------------------------------
//
// 因此，在解码时需要使用正确方式解析，可以使用rlp.Stream 依次解析。
func GetRawUnit(db ntsdb.Database, chain common.Address, uhash common.Hash, number uint64) rlp.RawValue {
	return append(
		GetHeaderRLP(db, uhash),
		GetUnitBodyRLP(db, chain, uhash, number)...)
}

// 解密单元信息
// 注意：不校验数据合法性
func DecodeRawUnit(b []byte) (*types.Unit, error) {
	return DecodeRawUnitStream(rlp.NewStream(bytes.NewReader(b), 0))
}
func DecodeRawUnitStream(stream *rlp.Stream) (*types.Unit, error) {
	header, err := decodeHeader(stream)
	if err != nil {
		return nil, err
	}
	body, err := DecodeUnitBody(stream)
	if err != nil {
		return nil, err
	}
	return types.NewUnitWithBody(header, body), nil
}

// 记录单元的原始交易集合
func WriteUnitOriginTxs(db ntsdb.Putter, uhash common.Hash, number uint64, txs []common.Hash) {
	if len(txs) == 0 {
		//说明无
		return
	}
	b, err := rlp.EncodeToBytes(txs)
	if err != nil {
		panic(err)
	}
	err = db.Put(unitOriginTxsKey(uhash, number), b)
	if err != nil {
		gameover(err, "failed to storage")
	}
}

func GetUnitOriginTxs(db ntsdb.Getter, chain common.Address, uhash common.Hash, number uint64) (types.Transactions, error) {
	list := GetUnitOringTxHash(db, uhash, number)
	txs := make(types.Transactions, len(list))
	for i := 0; i < len(list); i++ {
		tx := GetTx(db, list[i])
		if tx == nil {
			return nil, ErrNotFound
		}
		txs[i] = tx
	}
	return txs, nil
}

// GetUnit 通过单元获取完整单元数据，包括Payload，Payload为空则返回空
func GetUnit(db ntsdb.Getter, hash common.Hash) *types.Unit {
	// Retrieve the unit header and body contents
	header, err := GetHeader(db, hash)
	if err != nil {
		return nil
	}
	body := GetUnitBody(db, header.MC, hash, header.Number)
	if body == nil {
		return nil
	}
	// 组包成新单元对象
	return types.NewBlockWithHeader(header).WithBody(body.Txs, body.Votes, body.Sign)
}
