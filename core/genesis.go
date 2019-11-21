package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/spf13/viper"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis unit. It also defines hard
// fork switch-over units through the chain configuration.
type Genesis struct {
	Config      *params.ChainConfig `json:"config"`
	Timestamp   uint64              `json:"timestamp"`
	ExtraData   []byte              `json:"extraData"`
	GasLimit    uint64              `json:"gasLimit"   gencodec:"required"`
	Mixhash     common.Hash         `json:"mixHash"`
	MainAccount common.Address      `json:"mainAccount"`
	Alloc       GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis units.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

// GenesisAlloc specifies the initial state that is part of the genesis unit.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.Address]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis unit.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Timestamp math.HexOrDecimal64
	//ExtraData  hexutil.Bytes
	GasLimit math.HexOrDecimal64
	GasUsed  math.HexOrDecimal64
	Number   math.HexOrDecimal64
	Alloc    map[common.Address]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}

	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis unit with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database already contains an incompatible genesis unit (have %x, new %x)", e.Stored[:8], e.New[:8])
}

// SetupGenesisUnit writes or updates the genesis unit in db.
// The unit that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork unit below the local head unit). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisUnit(db ntsdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	// 检查是否有传入创世配置
	if genesis != nil && genesis.Config == nil {
		return params.AllProtocolChanges, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new unit if there is no stored genesis unit.

	// first record muse be genesis unit.
	// 去数据库获取创世单元哈希
	stored := GetStableHash(db, params.SCAccount, 0)

	// store genesis unit
	// 如果找不到在数据库的创世单元，就构建创世单元并存入
	if stored.Empty() {
		// 如果没传参，构建默认单元
		if genesis == nil {
			log.Info("Writing genesis from config")
			var err error
			genesis, err = LoadGenesisConfig()
			if err != nil {
				return nil, common.Hash{}, err
			}
		} else {
			log.Info("Writing custom genesis unit")
		}
		// 存储数据
		unit, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		hash := unit.Hash()
		return genesis.Config, hash, err
	}
	// 如果存储的数据不为空，继续校验创世数据是否相同
	if genesis != nil {

		// 构建创世数据
		unit, _, _, err := genesis.ToUnit()
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		hash := unit.Hash()
		// 检查差异
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}
	// Get the existing chain configuration.
	// 根据创世哈希重新取配置。暂时不清楚意图
	newcfg := genesis.configOrDefault(stored)
	// 从存储中取配置
	storedcfg, err := GetChainConfig(db, stored)
	if err != nil {
		// 如果没有存储则写入。暂时不清楚意图
		if err == ErrChainConfigNotFound {
			// This case happens if a genesis write was interrupted.
			log.Warn("Found genesis unit without chain config")
			err = WriteChainConfig(db, stored, newcfg)
		}
		return newcfg, stored, err
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	//
	// 如果没传参数，并且存储的数据和主网不同，优先使用存储的配置。暂时不清楚意图
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}

	//FixUnitStore(db.(ntsdb.Finder), db)

	// 获取最新的系统链单元
	hash := GetChainTailHash(db, params.SCAccount)
	header := GetHeaderByHash(db, hash)
	if header == nil {
		return newcfg, stored, fmt.Errorf("not found system chain last stabled header by hash %s", hash.Hex())
	}

	// 从state中读取最新的系统配置
	stateDB, err := state.New(header.MC.Bytes(), header.StateRoot, state.NewDatabase(db))
	if err != nil {
		return storedcfg, stored, err
	}

	for key := range newcfg.GetProposal() {
		value, err := sc.ReadConfig(stateDB, key)
		if err != nil {
			return storedcfg, stored, err
		}
		newcfg.Set(key, value)
	}

	return newcfg, stored, WriteChainConfig(db, stored, newcfg)
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil && g.Config != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	default:
		return params.AllProtocolChanges
	}
}

// ToBlock creates the unit and state of a genesis specification.
func (g *Genesis) ToUnit() (*types.Unit, *state.StateDB, types.Receipts, error) {

	//var msgs []types.Message
	head := &types.Header{
		MC:         params.SCAccount,
		Number:     g.Number,
		Timestamp:  g.Timestamp,
		ParentHash: g.ParentHash,
	}

	db, _ := ntsdb.NewMemDatabase()
	statedb, _ := state.New(head.MC.Bytes(), common.Hash{}, state.NewDatabase(db))
	//必须先写入创世配置
	if err := sc.SetupChainConfig(statedb, g.Config); err != nil {
		return nil, nil, nil, err
	}

	//虚拟一个交易哈希
	txHash := common.SafeHash256("NerthusGenesisInitTx")
	statedb.Prepare(txHash, common.Hash{}, 0, 0)

	allocs := make(common.AddressList, 0, len(g.Alloc))
	for addr := range g.Alloc {
		allocs = append(allocs, addr)
	}
	sort.Sort(allocs)

	for _, addr := range allocs {
		account := g.Alloc[addr]
		statedb.SetBalance(addr, big.NewInt(0)) //使得 coinflow 中可展示期初数据
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	if err := sc.SetupSystemWitness(statedb, DefaultSystemWitness()); err != nil {
		return nil, nil, nil, err
	}
	if err := sc.SetupCouncilMember(statedb, DefaultCouncilMember(), g.Config.Get(params.ConfigParamsCouncilCap)); err != nil {
		return nil, nil, nil, err
	}
	if err := sc.SetupUserWitness(statedb, DefaultUserWitness()); err != nil {
		return nil, nil, nil, err
	}
	// 为创世账户分配见证人
	users := make([]common.Address, allocs.Len())
	for i, addr := range allocs {
		users[i] = addr
	}
	if err := sc.SetupDefaultWitnessForUser(statedb, users...); err != nil {
		return nil, nil, nil, err
	}
	root, flows := statedb.IntermediateRoot(common.Address{})
	var coinflows types.Coinflows
	for _, v := range flows {
		coinflows = append(coinflows, types.Coinflow{
			To:     v.Address,
			Amount: new(big.Int).Set(&v.Diff),
		})
	}

	var usedGas uint64 = 1
	receipt := types.NewReceipt(types.ActionTransferReceipt, root, false, big.NewInt(0))
	receipt.Coinflows = coinflows
	receipt.GasUsed = usedGas
	receipt.TxHash = txHash
	receipt.GasUsed = usedGas
	receipt.GasRemaining = 0
	// 获取交易日志，并创建就一个 bloom 过滤器
	receipt.Logs = statedb.Logs()
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	head.StateRoot = root
	head.GasUsed = receipt.GasUsed
	unit := types.NewUnit(head, []*types.TransactionExec{{Action: types.ActionTransferReceipt}})
	//使用公开私钥对创世数据进行签名
	key, err := crypto.HexToECDSA(params.PublicPrivateKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load private key,%v", err)
	}
	signer := types.NewSigner(g.Config.ChainId)
	if err := types.SignBySignHelper(unit, signer, key); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to sign genesis unit,%v", err)
	}
	return unit, statedb, types.Receipts{receipt}, nil
}

// Commit writes the unit and state of a genesis specification to the database.
// The unit is committed as the canonical head unit.
func (g *Genesis) Commit(db ntsdb.Database) (*types.Unit, error) {

	ucWitNum := params.ConfigParamsUCWitness
	if count := len(DefaultUserWitness()); count < int(ucWitNum) {
		return nil, fmt.Errorf("can't commit genesis unit with few default witneesses, at least %d, actually only %d", ucWitNum, count)
	}

	unit, statedb, receipts, err := g.ToUnit()
	if err != nil {
		return nil, err
	}
	if unit.Number() != 0 {
		return nil, fmt.Errorf("can't commit genesis unit with number %d > 0", unit.Number())
	}
	if unit.MC() != params.SCAccount {
		return nil, fmt.Errorf("invalid chain address,should be %s", params.SCAccount)
	}
	if _, err := statedb.CommitTo(db); err != nil {
		return nil, fmt.Errorf("cannot write state: %v", err)
	}
	hash := unit.Hash()
	batch := db.NewBatch()
	defer batch.Discard()

	if err := WriteUnit(db, unit, receipts); err != nil {
		return nil, err
	}
	handleSpecialLogs(db, unit, receipts[0].Logs)
	WriteUnitReceipts(db, hash, unit.MC(), unit.Number(), receipts)
	WriteStableHash(batch, hash, unit.MC(), unit.Number())
	if err := WriteChainTailHash(batch, hash, unit.MC()); err != nil {
		return nil, err
	}

	// 将创世账户的单元标记为创世位置
	for addr := range g.Alloc {
		err := WriteChainTailHash(batch, hash, addr)
		if err != nil {
			return nil, err
		}
	}
	//记录系统链的Last
	if err := WriteChainTailHash(batch, hash, params.SCAccount); err != nil {
		return nil, err
	}

	config := g.Config
	if config == nil {
		config = params.AllProtocolChanges
	}
	if err := WriteChainConfig(batch, unit.Hash(), config); err != nil {
		return nil, err
	}

	return unit, batch.Write()
}

// MustCommit writes the genesis unit and state to db, panicking on error.
// The unit is committed as the canonical head unit.
func (g *Genesis) MustCommit(db ntsdb.Database) *types.Unit {
	unit, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return unit
}

// DefaultGenesisUnit returns the  main net genesis unit.
func DefaultGenesisUnit() *Genesis {
	return &Genesis{
		Config:   params.MainnetChainConfig,
		GasLimit: 5000,
		Alloc:    decodePrealloc(mainnetAllocData),
	}
}

// DevGenesisUnit returns the 'geth --dev' genesis unit.
func DevGenesisUnit() *Genesis {

	v := viper.New()
	config.InitViper(v, "cnts.dev")

	return &Genesis{
		Config:   params.TestChainConfig,
		GasLimit: 4712388,
		Alloc:    decodePrealloc(devAllocData),
	}
}

func decodePrealloc(data string) GenesisAlloc {
	var p []struct {
		Address string
		Balance *big.Int
	}
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.ForceDecodeAddress(account.Address)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}
