package arbitration

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

type TxHelper interface {
	SignTx(tx *types.Transaction) error
	BroadcastTx(tx *types.Transaction) error
	EventMux() *event.TypeMux
}
type Worker struct {
	chainr    ChainReader
	txHelper  TxHelper
	db        ntsdb.Database
	badChainc chan common.Address
	witness   common.Address

	wk   *sync.WaitGroup
	quit chan struct{}
}

func New(cr ChainReader, db ntsdb.Database, txHelper TxHelper, witness common.Address) (*Worker, error) {
	return &Worker{
		chainr:    cr,
		db:        db,
		txHelper:  txHelper,
		badChainc: make(chan common.Address, 36),
		quit:      make(chan struct{}),
		wk:        &sync.WaitGroup{},
		witness:   witness,
	}, nil
}

func (w *Worker) Start() error {
	go w.do()
	log.Info("stated system witness arbitration server", "witness", w.witness)
	return nil
}

func (w *Worker) Stop() {
	log.Trace("stoping system witness arbitration server", "witness", w.witness)
	close(w.quit)
	w.wk.Wait()
	log.Info("stopped system witness arbitration server", "witness", w.witness)
}

// 为了防止丢失，系统见证人必须一直监听需要投票工作，以便让链恢复稳定
// 监听时，先扫描尚未处理的交易，并将其选择一个合法单元后，广播此交易参与投票

func (w *Worker) do() {
	w.wk.Add(1)
	defer w.wk.Done()

	checkingTODO := func() {
		//如果已满，则不需要拉取数据
		if len(w.badChainc) == cap(w.badChainc) {
			return
		}

		chains := GetAllBadChain(w.db)
		for _, v := range chains {
			log.Info("bad chain", "chain", v)
			select {
			case w.badChainc <- v:
			default:
				//如果已满则放弃，避免压入太多数据而出现堵塞，因为badChainc的处理也在此For中
			}
		}
	}

	//订阅消息
	sub := badUnitEventBus.Subscribe(w.badChainc)
	defer sub.Unsubscribe()

	//启动时检查一次
	checkingTODO()
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()
	for {
		select {
		case <-w.quit:
			return
		case <-tk.C:
			checkingTODO()
		case bad := <-w.badChainc:
			//获取
			err := w.handleBadChain(bad)
			if err != nil {
				log.Error("failed to handle bad chain", "chain", bad, "err", err)
			} else {
				log.Info("handle bad chain done", "chain", bad)
			}
		}
	}
}

//如果本地依旧存在此数据，表明该链还尚未恢复。
// 不能恢复的原因是：见证人没有投票。
// 而没有投票则是因为：1. 无法选择合法单元 2.尚未发现作恶。3.投票未能达成一致。
// 1. 无法选择合法单元：是因为没得选，此时不能恢复。但这不影响作恶高度之前的单元落地。
// 2. 尚未发现作恶：可以忽略，所有系统见证人均会不断检查和广播作恶信息。
// 3. 投票未能达成一致：  TODO: 只有超过+1的投票才能成功，为了防止出现僵局，需要采用BFT机制线下达成共识。
// 当前做法：投票需要在1小时内结束，如果未能达成一致，需要重新投票（这样为以后优化留下空间）。直到达成一致。
func (w *Worker) handleBadChain(chain common.Address) error {
	//获取作恶信息
	badFlag := GetBadChainFlag(w.db, chain)
	if badFlag.Number == 0 {
		return errors.New("missing bad unit flag info")
	}
	foundTime := time.Unix(int64(badFlag.FoundTimestamp), 0)

	logger := log.New("chain", chain, "number", badFlag.Number, "found_time", foundTime)
	logger.Info("processing bad unit")

	//检查我是否已经投票
	tx := getMyChooseVoteTx(w.db, badFlag.Chain, badFlag.Number)
	if tx != nil && !tx.Expired(time.Now()) {
		//不需要重新创建，只需要广播该交易
		if err := w.txHelper.BroadcastTx(tx); err != nil {
			return err
		}
		return nil
	}
	//删除过期存储
	if tx != nil {
		deleteMyChooseVoteTx(w.db, badFlag.Chain, badFlag.Number)
	}
	//选择合法单元
	choose, err := w.ChooseGoodOne(badFlag.Chain, badFlag.Number)
	if err != nil {
		return err
	}
	if choose == nil {
		logger.Warn("can not choose good unit from bad chain", "")
		return nil
	}

	//获取已存储的证据
	proof := GetBadWitnessProof(w.db, badFlag.ProofID)
	if len(proof) == 0 {
		return errors.New("missing bad witness proof data")
	}

	//重新广播此信息到网络
	broadcastBadWitnessProof(w.txHelper.EventMux(), proof)

	{
		//创建交易
		inputData, err := sc.CreateReportDishonestTxInput(*types.CopyHeader(choose), proof)
		if err != nil {
			return err
		}
		tx := types.NewTransaction(w.witness, params.SCAccount, big.NewInt(0), 0, 0, types.DefTxMaxTimeout, inputData)
		if err := w.txHelper.SignTx(tx); err != nil {
			return err
		}

		//广播交易
		if err := w.txHelper.BroadcastTx(tx); err != nil {
			return err
		}
		//存储交易
		writeMyChooseVoteTx(w.db, badFlag.Chain, badFlag.Number, tx)
		logger.Info("my choose", "chain", badFlag.Chain, "witness", w.witness, "uhash", choose.Hash(), "txHash", tx.Hash())
	}
	return nil
}

// 系统见证人在发现作恶后需要选择出一个合法单元，只有在能选择出合法单元时才进行合法单元投票。
func (w *Worker) ChooseGoodOne(chain common.Address, number uint64) (*types.Header, error) {
	//先查找出本地存在的单元，只有存在才能说明此单元合法
	tail := w.chainr.GetChainTailHead(chain)
	//当本地该链的末尾单元还尚未到达此高度时，必然无法选出合法。
	if tail.Number < number { //
		// 此时不能恢复。但这不影响作恶高度之前的单元落地
		return nil, fmt.Errorf("missing unit at chain")
	}
	//此时说明本地存在该位置单元，发起投票
	good := w.chainr.GetHeaderByNumber(chain, number)
	if good == nil {
		return nil, errors.New("missing unit")
	}
	return good, nil
}
