// @Author: yin

package ntsapi

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/consensus/ethash"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

const (
	defaultGas   = 90000
	defaultSpeed = 1
	//defaultGasPrice = 50 * params.Shannon
)

// PublicNerthusAPI provides an API to access Nerthus related information.
// It offers only methods that operate on public data that is freely available to anyone.
type PublicNerthusAPI struct {
	b Backend
}

// NewPublicNerthusAPI creates a new Nerthus protocol API.
func NewPublicNerthusAPI(b Backend) *PublicNerthusAPI {
	return &PublicNerthusAPI{b}
}

// ProtocolVersion returns the current Nerthus protocol version this node supports
func (s *PublicNerthusAPI) ProtocolVersion() hexutil.Uint {
	return hexutil.Uint(s.b.ProtocolVersion())
}

// PublicTxPoolAPI offers and API for the transaction pool. It only operates on data that is non confidential.
type PublicTxPoolAPI struct {
	b Backend
}

// NewPublicTxPoolAPI creates a new tx pool service that gives information about the transaction pool.
func NewPublicTxPoolAPI(b Backend) *PublicTxPoolAPI {
	return &PublicTxPoolAPI{b}
}

// PrivateAccountAPI provides an API to access accounts managed by this node.
// It offers methods to create, (un)lock en list accounts. Some methods accept
// passwords and are therefore considered private by default.
type PrivateAccountAPI struct {
	am        *accounts.Manager
	nonceLock *AddrLocker
	b         Backend
	powAddr   common.Address
	powErr    error
	powIng    bool
}

// NewPrivateAccountAPI create a new PrivateAccountAPI.
func NewPrivateAccountAPI(b Backend, nonceLock *AddrLocker) *PrivateAccountAPI {
	return &PrivateAccountAPI{
		am:        b.AccountManager(),
		nonceLock: nonceLock,
		b:         b,
	}
}

// ListAccounts will return a list of addresses for accounts this node manages.
func (s *PrivateAccountAPI) ListAccounts() []common.Address {
	addresses := make([]common.Address, 0) // return [] instead of nil if empty
	for _, wallet := range s.am.Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

// rawWallet is a JSON representation of an accounts.Wallet interface, with its
// data contents extracted into plain fields.
type rawWallet struct {
	URL      string             `json:"url"`
	Status   string             `json:"status"`
	Failure  string             `json:"failure,omitempty"`
	Accounts []accounts.Account `json:"accounts,omitempty"`
}

// ListWallets will return a list of wallets this node manages.
func (s *PrivateAccountAPI) ListWallets() []rawWallet {
	wallets := make([]rawWallet, 0) // return [] instead of nil if empty
	for _, wallet := range s.am.Wallets() {
		status, failure := wallet.Status()

		raw := rawWallet{
			URL:      wallet.URL().String(),
			Status:   status,
			Accounts: wallet.Accounts(),
		}
		if failure != nil {
			raw.Failure = failure.Error()
		}
		wallets = append(wallets, raw)
	}
	return wallets
}

// OpenWallet initiates a hardware wallet opening procedure, establishing a USB
// connection and attempting to authenticate via the provided passphrase. Note,
// the method may return an extra challenge requiring a second open (e.g. the
// Trezor PIN matrix challenge).
func (s *PrivateAccountAPI) OpenWallet(url string, passphrase *string) error {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return err
	}
	pass := ""
	if passphrase != nil {
		pass = *passphrase
	}
	return wallet.Open(pass)
}

// DeriveAccount requests a HD wallet to derive a new account, optionally pinning
// it for later reuse.
func (s *PrivateAccountAPI) DeriveAccount(url string, path string, pin *bool) (accounts.Account, error) {
	wallet, err := s.am.Wallet(url)
	if err != nil {
		return accounts.Account{}, err
	}
	derivPath, err := accounts.ParseDerivationPath(path)
	if err != nil {
		return accounts.Account{}, err
	}
	if pin == nil {
		pin = new(bool)
	}
	return wallet.Derive(derivPath, *pin)
}

// NewAccount will create a new account and returns the address for the new account.
func (s *PrivateAccountAPI) NewAccount(password string) (common.Address, error) {
	acc, err := fetchKeystore(s.am).NewAccount(password)
	if err == nil {
		return acc.Address, nil
	}
	return common.Address{}, err
}

// fetchKeystore retrives the encrypted keystore from the account manager.
func fetchKeystore(am *accounts.Manager) *keystore.KeyStore {
	return am.Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
}

// ImportRawKey stores the given hex encoded ECDSA key into the key directory,
// encrypting it with the passphrase.
func (s *PrivateAccountAPI) ImportRawKey(privkey string, password string) (common.Address, error) {
	key, err := crypto.HexToECDSA(privkey)
	if err != nil {
		return common.Address{}, err
	}
	acc, err := fetchKeystore(s.am).ImportECDSA(key, password)
	return acc.Address, err
}

// 导入 Keystore，需要提供原密码和新密码
func (s *PrivateAccountAPI) ImportKeystore(keyJSON, passphrase, newPassphrase string) (common.Address, error) {
	acc, err := fetchKeystore(s.am).Import([]byte(keyJSON), passphrase, newPassphrase)
	return acc.Address, err
}

// UnlockAccount will unlock the account associated with the given address with
// the given password for duration seconds. If duration is nil it will use a
// default of 300 seconds. It returns an indication if the account was unlocked.
func (s *PrivateAccountAPI) UnlockAccount(addr common.Address, password string, duration *uint64) (bool, error) {
	const max = uint64(time.Duration(math.MaxInt64) / time.Second)
	var d time.Duration
	if duration == nil {
		d = 300 * time.Second
	} else if *duration > max {
		return false, errors.New("unlock duration too large")
	} else {
		d = time.Duration(*duration) * time.Second
	}
	err := fetchKeystore(s.am).TimedUnlock(accounts.Account{Address: addr}, password, d)
	return err == nil, err
}

// LockAccount will lock the account associated with the given address when it's unlocked.
func (s *PrivateAccountAPI) LockAccount(addr common.Address) bool {
	return fetchKeystore(s.am).Lock(addr) == nil
}

// SendTransaction will create a transaction from the given arguments and
// tries to sign it with the key associated with args.To. If the given passwd isn't
// able to decrypt the key it fails.
// Author: @kulics 发送交易
func (s *PrivateAccountAPI) SendTransaction(ctx context.Context, args SendTxArgs, passwd string) (common.Hash, error) {
	account := accounts.Account{Address: args.From}
	wallet, err := s.am.Find(account)
	if err != nil {
		return common.Hash{}, err
	}

	// Set some sanity defaults and terminate on failure
	if err := args.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}
	// Assemble the transaction and sign with the wallet
	tx, err := args.toTransaction(s.b)
	if err != nil {
		return common.Hash{}, err
	}
	_, err = wallet.SignHelperWithPassphrase(account, passwd, tx, s.b.ChainConfig().ChainId)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, tx)
}

// SendWitnessPowTxRealTime 发送更换见证人交易
func (s *PrivateAccountAPI) SendWitnessPowTxRealTime(addr, chain common.Address, passwd string) (string, error) {
	if chain.Empty() {
		return "", errors.New("chain address is empty")
	}
	if chain != addr && !chain.IsContract() { //如果不是更换合约地址则提示错误
		return "", errors.New("can not apply change other user witness")
	}
	if !s.powAddr.Empty() {
		return "", errors.New("pow computing, can't again")
	}
	account := accounts.Account{Address: addr}
	wallet, err := s.am.Find(account)
	if err != nil {
		return "", err
	}
	changeWitnessAddr := params.SCAccount
	// 获取链状态,根据链状态判断是否是申请见证人还是更换见证人
	status, err := s.b.DagChain().GetChainStatus(chain)
	if err != nil {
		return "", err
	}
	var inputData []byte
	switch status {
	case sc.ChainStatusNotWitness:
		inputData = sc.CreateCallInputData(sc.FuncApplyWitnessID, chain)
	case sc.ChainStatusNormal, sc.ChainStatusInsufficient:
		inputData = sc.CreateCallInputData(sc.FuncReplaceWitnessID, chain)
	case sc.ChainStatusWitnessReplaceUnderway:
		//但允许超时重发
		db, err := s.b.DagChain().GetChainTailState(params.SCAccount)
		if err != nil {
			return "", err
		}
		_, number := sc.GetChainStatus(db, chain)
		if !sc.GetChainStatusIsTimeout(number, s.b.DagChain().GetChainTailNumber(params.SCAccount)) {
			return "", errors.New("witness are being replaced")
		}
		inputData = sc.CreateCallInputData(sc.FuncReplaceWitnessID, chain)
	default:
		return "", errors.New("inconsistent chain status")
	}

	// 预先模拟执行交易签名,如果失败则不需要pow计算
	if _, err = wallet.SignHashWithPassphrase(account, passwd, common.StringToHash("hash").Bytes()); err != nil {
		return "", err
	}
	tx := types.NewTransaction(addr, changeWitnessAddr, big.NewInt(0), 0, 0, types.DefTxTimeout, inputData)
	if s.b.TxPool().ExistFeeTx(tx.Data4(), addr) {
		return "", errors.New("a free transaction is processing,can not send new")
	}
	//预执行一次
	header := s.b.DagChain().GetChainTailHead(params.SCAccount)
	state, err := s.b.DagChain().StateAt(params.SCAccount, header.StateRoot)
	if err != nil {
		return "", err
	}
	msg := types.NewMessage(tx, tx.Gas(), addr, params.SCAccount, false)
	txExec := &types.TransactionExec{Action: types.ActionPowTxContractDeal, TxHash: tx.Hash(), Tx: tx}
	_, err = core.ProcessTransaction(s.b.ChainConfig(), s.b.DagChain(), state, header, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), simulationConfig(), msg)
	if err != nil {
		return "", fmt.Errorf("local simulation execution failed,%s", err)
	}
	s.powAddr = chain
	s.powIng = true
	s.powErr = nil
	go func() {
		log.Debug("replace witness pow computing", "addr", addr)
		tx := types.NewTransaction(addr, changeWitnessAddr, big.NewInt(0), 0, 0, types.DefTxTimeout, inputData)

		stopc := make(chan struct{})
		seedCh := ethash.ProofWork(tx, 1, s.b.ChainConfig().PowLimit(), stopc)
		defer func(begin time.Time) {
			if s.powErr != nil {
				log.Error("replace witness tx submit failed", "err", s.powErr)
			}
			s.powIng = false
			s.powAddr = common.Address{}
			s.powErr = nil
			log.Info("create replace witness tx done", "elapsed", time.Since(begin))
		}(time.Now())

		// 超时/退出处理
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt, syscall.SIGKILL, syscall.SIGTERM)
		select {
		case seed := <-seedCh:
			tx.SetSeed(seed)
			log.Debug("replace witness pow over", "addr", addr, "seed", seed, "hash", tx.Hash())
		case <-sigc:
			stopc <- struct{}{}
			return
		case <-time.After(time.Minute * 60):
			//不会设置超过 1小时的计算难道。因此这里是有一小时作为最长时间。
			//在公测网络中，基本需要半小时的计算。
			stopc <- struct{}{}
			s.powErr = errors.New("work timeout")
			return
		}

		if _, s.powErr = wallet.SignHelperWithPassphrase(account, passwd, tx, s.b.ChainConfig().ChainId); s.powErr == nil {
			_, s.powErr = submitTransaction(nil, s.b, tx)
		}
	}()
	return "computing", nil
}

// GetPowResult 获取pow交易结果
func (s *PrivateAccountAPI) GetPowResult(addr common.Address) (bool, error) {
	if s.powAddr.Empty() {
		if addr != s.powAddr {
			return false, nil
		}
		if s.powErr != nil {
			return true, s.powErr
		}
		return false, nil
	}
	return true, nil
}

// GetPowAddr 获取当前正在计算的
func (s *PrivateAccountAPI) GetPowAddr() (common.Address, error) {
	return s.powAddr, nil
}

// 获取账户状态：
//  powing: PoW计算中
//  noraml: 正常
//  	  ： 更换见证人中
func (s *PrivateAccountAPI) GetAccountStatus(addr common.Address) string {
	//如果正在执行 Pow 计算，则显示
	if addr == s.powAddr && s.powIng {
		return "powing"
	}

	db, err := s.b.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return sc.ChainStatusNormal.String()
	}
	status, _ := sc.GetChainStatus(db, addr)
	//如果不是正常状态，则显示
	if status != sc.ChainStatusNormal && status != sc.ChainStatusNotWitness {
		return status.String()
	}
	//如果是正常状态，则检查是否有在途的交易，涉及更换见证人
	txs := s.b.TxPool().PendingByAcct(addr)
	for _, tx := range txs {
		if tx.To() == params.SCAccount {
			who, yes := sc.IsCallReplaceWitness(tx.Data())
			if yes && who == addr {
				return sc.ChainStatusWitnessReplaceUnderway.String()
			}
		}
	}
	//如果是正常状态则检查人数是否充足
	if status == sc.ChainStatusNormal {
		gpIndex, err := sc.GetGroupIndexFromUser(db, addr)
		if err == nil {
			count := sc.GetWitnessCountAt(db, gpIndex)
			if count < uint64(params.GetChainMinVoting(addr)) {
				return sc.ChainStatusInsufficient.String()
			}
		}
	}
	return status.String()
}

// submitTransaction is a helper function that submits tx to txPool and logs a message.
func submitTransaction(ctx context.Context, b Backend, tx *types.Transaction) (common.Hash, error) {
	// 发送交易信息
	err := b.SendTransaction(tx)
	if err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

func (s *PrivateAccountAPI) GetCouncilHeartbeat(address common.Address, period uint64) (interface{}, error) {
	hc, err := s.b.DagChain().GetCouncilHeartbeat()
	if err != nil {
		return nil, err
	}
	if period <= 0 {
		period = s.b.DagChain().GetPeriod()
	}
	addrHc := hc.Query(address)
	if addrHc.Address == common.EmptyAddress {
		return nil, nil
	}
	type resultType struct {
		Address    common.Address
		LastTurn   uint64
		LastNumber uint64
		Count      uint64
	}
	ret := resultType{
		Address:    address,
		LastTurn:   addrHc.LastTurn,
		LastNumber: addrHc.LastNumber,
		Count:      0,
	}
	//b, e := addrHc.Count.GetWorkPeriod()
	//for i := b; i <= e; i++ {
	//	ret.Count = append(ret.Count, addrHc.Count.Query(i))
	//}
	ret.Count = addrHc.Count.Query(period).UnitCount

	return ret, nil
}

/// 重新广播某笔交易
func (api *PrivateAccountAPI) ReBroadcastTx(txHash common.Hash) error {
	tx := api.b.TxPool().Get(txHash)
	if tx == nil {
		tx = api.b.DagChain().GetTransaction(txHash)
	}
	if tx == nil {
		return errors.New("local not exists the transaction")
	}
	return api.b.TxPipe().SendTransaction(types.Transactions{tx})
}

// PublicTransactionPoolAPI exposes methods for the RPC interface
type PublicTransactionPoolAPI struct {
	b         Backend
	nonceLock *AddrLocker
}

// NewPublicTransactionPoolAPI creates a new RPC service with methods specific for the transaction pool.
func NewPublicTransactionPoolAPI(b Backend, nonceLock *AddrLocker) *PublicTransactionPoolAPI {
	return &PublicTransactionPoolAPI{b, nonceLock}
}

// SendTransaction creates a transaction for the given argument, sign it and submit it to the
// transaction pool.
func (s *PublicTransactionPoolAPI) SendTransaction(ctx context.Context, args SendTxArgs) (common.Hash, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: args.From}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return common.Hash{}, err
	}

	// 设置默认值
	//if args.Nonce == nil {
	//	// Hold the addresse's mutex around signing to prevent concurrent assignment of
	//	// the same nonce to multiple accounts.
	//	s.nonceLock.LockAddr(args.From)
	//	defer s.nonceLock.UnlockAddr(args.From)
	//}

	// Set some sanity defaults and terminate on failure
	if err := args.setDefaults(ctx, s.b); err != nil {
		return common.Hash{}, err
	}
	// Assemble the transaction and sign with the wallet
	tx, err := args.toTransaction(s.b)
	if err != nil {
		return common.Hash{}, err
	}

	signed, err := wallet.SignTx(account, tx, s.b.ChainConfig().ChainId)
	if err != nil {
		return common.Hash{}, err
	}
	return submitTransaction(ctx, s.b, signed)
}

// SendRawTransaction will add the signed transaction to the transaction pool.
// The sender is responsible for signing the transaction and using the correct nonce.
func (s *PublicTransactionPoolAPI) SendRawTransaction(ctx context.Context, encodedTx hexutil.Bytes) (common.Hash, error) {
	tx := new(types.Transaction)
	if err := rlp.DecodeBytes(encodedTx, tx); err != nil {
		return common.Hash{}, err
	}
	//return submitTransaction(ctx, s.b, tx)
	if err := s.b.SendRemoteTransaction(tx); err != nil {
		return tx.Hash(), err
	}
	return tx.Hash(), nil
}

// Sign calculates an ECDSA signature for:
// keccack256("\x19Nerthus Signed Message:\n" + len(message) + message).
//
// Note, the produced signature conforms to the secp256k1 curve R, S and V values,
// where the V value will be 27 or 28 for legacy reasons.
//
// The account associated with addr must be unlocked.
//
// https://github.com/Nerthus/wiki/wiki/JSON-RPC#eth_sign
func (s *PublicTransactionPoolAPI) Sign(addr common.Address, data hexutil.Bytes) (hexutil.Bytes, error) {
	// Look up the wallet containing the requested signer
	account := accounts.Account{Address: addr}

	wallet, err := s.b.AccountManager().Find(account)
	if err != nil {
		return nil, err
	}
	// Sign the requested hash with the wallet
	signature, err := wallet.SignHash(account, signHash(data))
	if err == nil {
		signature[64] += 27 // Transform V from 0/1 to 27/28 according to the yellow paper
	}
	return signature, err
}

type SendTxArgs struct {
	From     common.Address `json:"from"`
	To       common.Address `json:"to"`
	Gas      *hexutil.Big   `json:"gas"`
	GasPrice *hexutil.Big   `json:"gasPrice"`
	Amount   *hexutil.Big   `json:"amount"`
	Data     hexutil.Bytes  `json:"data"`
	Input    hexutil.Bytes  `json:"input"`
	Timeout  *hexutil.Big   `json:"timeout"`
}

// init 交易内容初始化
func (args *SendTxArgs) init(ctx context.Context, b Backend) error {
	if err := args.setDefaults(ctx, b); err != nil {
		return err
	}
	//使用单元Gas上限
	args.Gas = (*hexutil.Big)(new(big.Int).SetUint64(b.ChainConfig().Get(params.ConfigParamsUcGasLimit)))
	return nil
}

// setDefaults 设置默认值
func (args *SendTxArgs) setDefaults(ctx context.Context, b Backend) error {
	if args.Gas == nil {
		args.Gas = (*hexutil.Big)(big.NewInt(defaultGas))
	}
	if args.From.Empty() {
		return errors.New("transaction from can't be empty")
	}
	if args.To.Empty() {
		return errors.New("transaction to can't be empty")
	}
	if args.GasPrice == nil || args.GasPrice.ToInt().Uint64() == 0 {
		price := b.ChainConfig().Get(params.ConfigParamsLowestGasPrice)
		args.GasPrice = (*hexutil.Big)(new(big.Int).SetUint64(price))
	}
	if args.Amount == nil {
		args.Amount = new(hexutil.Big)
	}
	if args.Timeout == nil {
		args.Timeout = (*hexutil.Big)(new(big.Int).SetUint64(types.DefTxMaxTimeout))
	}
	return nil

}

// toTransaction 将发送参数转化为交易
func (args *SendTxArgs) toTransaction(nts Backend) (*types.Transaction, error) {
	// 检测余额
	// Author: @kulics
	out := args.Amount.ToInt()
	balance, err := nts.DagReader().GetBalanceByAddress(args.From)
	if err != nil {
		return nil, err
	}
	if balance.Cmp(out) < 0 {
		return nil, fmt.Errorf("did not have enough balance, need %d > %d", out, balance)
	}
	return types.NewTransaction(args.From, args.To, (*big.Int)(args.Amount), args.Gas.ToInt().Uint64(), args.GasPrice.ToInt().Uint64(), args.Timeout.ToInt().Uint64(), args.Data), nil
}

// PublicNetAPI offers network related RPC methods
type PublicNetAPI struct {
	net            *p2p.Server
	networkVersion uint64
}

// NewPublicNetAPI creates a new net API instance.
func NewPublicNetAPI(net *p2p.Server, networkVersion uint64) *PublicNetAPI {
	return &PublicNetAPI{net, networkVersion}
}

// Listening returns an indication if the node is listening for network connections.
func (s *PublicNetAPI) Listening() bool {
	return true // always listening
}

// PeerCount returns the number of connected peersF
func (s *PublicNetAPI) PeerCount() hexutil.Uint {
	return hexutil.Uint(s.net.PeerCount())
}

// Version returns the current Nerthus protocol version.
func (s *PublicNetAPI) Version() string {
	return fmt.Sprintf("%d", s.networkVersion)
}

// signHash is a helper function that calculates a hash for the given message that can be
// safely used to calculate a signature from.
//
// The hash is calulcated as
//   keccak256("\x19Nerthus Signed Message:\n"${message length}${message}).
//
// This gives context to the signed message and prevents signing of transactions.
func signHash(data []byte) []byte {
	msg := fmt.Sprintf("\x19Nerthus Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256([]byte(msg))
}
