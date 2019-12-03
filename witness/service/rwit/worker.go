package rwit

import (
	"fmt"
	"time"

	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/rlp"

	"gitee.com/nerthus/nerthus/core"

	"gitee.com/nerthus/nerthus/nts/protocol"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/ntsdb"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

const (
	maxPending        = 30                         //运行内存总处理的见证人地址数量，超过数量的处理将不会被处理。
	defSleepNumbers   = 1                          //不能立即处理，需要等待片刻。防止出现投票结果不是最后单元的情况
	chooseVoteMsgCode = protocol.ChooseLastUnitMsg //消息 Code
)

type Worker struct {
	backend Backend
	signer  types.Signer
	chainer Chainer
	db      ntsdb.Database

	store   *PendingChainSet
	haveJob bool //是否存在需要投票的链，不想要控制并发

	dataSet *ChainDataSet       //投票存储
	pended  chan common.Address //正在进行中的链

	logger log.Logger
	quite  chan struct{}
}

type Job PendingChain

func New(backend Backend, chainer Chainer, db ntsdb.Database, signer types.Signer) *Worker {
	w := Worker{
		signer:  signer,
		chainer: chainer,
		backend: backend,
		db:      db,
		pended:  make(chan common.Address, maxPending),
		quite:   make(chan struct{}),
		store:   newPendingChainSet(db),
		dataSet: NewVoteSet(signer),
		logger:  log.New("module", "chooseLast"),
		haveJob: true,
	}
	return &w
}

func (w *Worker) Start() {
	w.logger.Debug("start")
	go w.listen()

}
func (w *Worker) Stop() {
	close(w.quite)
	w.logger.Debug("stopped")
}

// 获取更换见证人处理状态报表
func (w *Worker) StatusReport() interface{} {

	type Item struct {
		Chain PendingChain
		Votes []common.Address
	}

	var report struct {
		Stored  int
		Chains  map[common.Address]Item
		Working []common.Address
	}
	report.Chains = make(map[common.Address]Item)
	w.store.Range(func(chain PendingChain) bool {
		report.Chains[chain.Chain] = Item{
			Chain: chain,
		}
		return true
	})
	report.Working = w.dataSet.Chains()
	for _, addr := range report.Working {
		if info, ok := w.dataSet.GetChainData(addr); ok {
			report.Chains[addr] = Item{
				Chain: report.Chains[addr].Chain,
				Votes: info.Voters(),
			}
		}
	}

	report.Stored = len(report.Chains)
	return report
}

// 尝试加入新任务
func (w *Worker) tryAddJob(job Job) bool {
	if w.dataSet.Len() > maxPending {
		return false
	}
	//尚未到时间
	if w.chainer.GetChainTailNumber(params.SCAccount)-job.SysUnitNumber < defSleepNumbers {
		return false
	}

	w.dataSet.Replace(job.Chain, job)
	select {
	case w.pended <- job.Chain:
	default:
	}
	w.logger.Trace("push new chain job done", "chain", job.Chain, "number", job.SysUnitNumber)

	return true
}

func (w *Worker) loadNewJob(update ...bool) {
	if !w.haveJob {
		return
	}
	if w.dataSet.Len() > maxPending {
		return
	}
	w.haveJob = false //清理标记

	var db *state.StateDB
	if len(update) > 0 && update[0] {
		var err error
		db, err = w.chainer.GetChainTailState(params.SCAccount)
		if err != nil {
			w.logger.Error("get chain tail stata failed", "err", err)
		}
	}

	//从存储中加载足够多
	w.store.Range(func(info PendingChain) bool {
		w.haveJob = true //只要能从存储中查询到数据，说明有待处理

		//不重复
		if w.dataSet.Contains(info.Chain) {
			return false
		}
		//如果需要更新，则查询到正确内容，并检查是否需要更新
		if db != nil {
			s, number := sc.GetChainStatus(db, info.Chain)
			switch s {
			case sc.ChainStatusNormal:
				w.remove(info.Chain)
				return true
			case sc.ChainStatusWitnessReplaceUnderway:
				//以最新位置为准
				if number > info.SysUnitNumber {
					uhash := core.GetStableHash(w.db, params.SCAccount, number)
					if uhash.Empty() {
						w.logger.Error("missing unit", "chain", params.SCAccount, "number", number)
						return true
					}
					w.handleNewChain(info.Chain, types.UnitID{
						ChainID: params.SCAccount,
						Height:  number,
						Hash:    uhash,
					})
				}
			}

		}

		ok := w.tryAddJob(Job(info))
		w.logger.Trace("load job", "chain", info.Chain, "number", "number", "ok", ok)
		return ok
	})
}

func (w *Worker) handleChainDone(chain common.Address) {
	w.remove(chain)
	//尝试加载新
	w.loadNewJob()
}

func (w *Worker) remove(chain common.Address) {
	//如果有删除旧数据，则说明需要停止对当前链的工作
	w.store.Del(chain)
	w.dataSet.Remove(chain) //清理内存和已知的投票
}

func (w *Worker) handleNewChain(chain common.Address, unitID types.UnitID) {
	w.remove(chain)
	w.store.Add(chain, unitID)
	w.haveJob = true //必然有

	//尝试写入
	ok := w.tryAddJob(Job{Chain: chain, SysUnitHash: unitID.Hash, SysUnitNumber: unitID.Height})

	w.logger.Trace("chain need vote for repalce witness", "chain", chain, "number", unitID.Height, "pending", ok)
}

func (w *Worker) handleChainEvent(ev types.ChainEvent) error {

	if ev.Unit.MC() != params.SCAccount {
		//如果链在处理中，需要清理数据，因为标识该链存在新高度，目前属于合法，需要重新评估
		//这里会存在性能瓶颈吗？
		//     1. O(1)，但存在 Address 转 string 操作
		//     2. Remove 内部带锁
		w.dataSet.Remove(ev.Unit.MC())

		return nil
	}

	var checked bool
	for _, el := range ev.Logs {
		if !el.ContainsTopic(sc.GetEventTopicHash(sc.ChainStatusChangedEvent)) {
			continue
		}
		info, err := sc.UnpackChainStatusChangedEvent(el.Data)
		if err != nil {
			panic(fmt.Errorf("unpack event data failed:%v", err))
		}
		checked = true
		switch info.Status {
		case sc.ChainStatusNormal:
			w.handleChainDone(info.Chain)
		case sc.ChainStatusWitnessReplaceUnderway:
			w.handleNewChain(info.Chain, ev.Unit.ID())
		}
	}

	if !checked {
		//系统链有新单元时需要检查，因为一些因为时间问题无法处理部分将进行
		w.loadNewJob()
	}

	return nil
}

func (w *Worker) listen() {

	// 检查连接状态
	if online, less := w.backend.WitnessSet().Len(), params.GetChainMinVoting(params.SCAccount)-1; online < less { //如果无任何连接则不需要处理
		witnessc := make(chan types.WitnessConnectStatusEvent, params.ConfigParamsSCWitness)
		sub := w.backend.WitnessSet().SubChange(witnessc)

		//等待连接数足够
		for {
			if w.backend.WitnessSet().Len() >= less {
				sub.Unsubscribe()
				break
			}
			select {
			case <-w.quite:
				sub.Unsubscribe()
				return
			case <-witnessc:
			}
		}
		w.logger.Trace("online witness is ok")
	}

	//首次加载数据,需要进行一次全量更新,但这里只会更新最近数据
	w.loadNewJob(true)

	//参与投票
	tk := time.NewTicker(time.Second * params.OneNumberHaveSecondBySys)
	defer tk.Stop()

	eventc := make(chan types.ChainEvent, 2000) //否则容易引起堵塞
	sub2 := w.chainer.SubscribeChainEvent(eventc)
	defer sub2.Unsubscribe()

	for {
		select {
		case <-w.quite:
			//退出前保留所有
			for {
				if len(eventc) == 0 {
					break
				}
				ev := <-eventc
				err := w.handleChainEvent(ev)
				if err != nil {
					w.logger.Error("handle chain event failed", "unit", ev.Unit.Hash(), "err", err)
				}
			}
			return
		case chain := <-w.pended:
			w.doMyChoose(chain, false)
		case ev := <-eventc:
			err := w.handleChainEvent(ev)
			if err != nil {
				w.logger.Error("handle chain event failed", "unit", ev.Unit.Hash(), "err", err)
			}
			//因为事件频次高，大概率阻挡<-tk.C 处理，因此需要额外检查一次
			select {
			case <-tk.C:
				w.logger.Trace("checking job", "count", w.dataSet.Len())
				//定期检查，自己已投票但尚未接收到其他人的投票，则重新广播
				w.checkVoted()
			default:
			}
		case <-tk.C:
			w.logger.Trace("checking job", "count", w.dataSet.Len())
			//定期检查，自己已投票但尚未接收到其他人的投票，则重新广播
			w.checkVoted()
		}
	}
}

func (w *Worker) checkVoted() {
	chains := w.dataSet.Chains()
	for _, c := range chains {
		//重新处理
		w.doMyChoose(c, true)
	}
	//如果还更多可以处理则加载
	w.loadNewJob()

}

// 获取或创建自己的投票
// 返回： True 表示新创建投票，如果为 FALSE，则表示从缓存中获取的旧投票。
func (w *Worker) getOrCreateMyChoose(chain common.Address) (*types.ChooseVote, bool) {
	info, ok := w.dataSet.GetChainData(chain)
	if !ok {
		return nil, false
	}
	if ok {
		if v := info.Vote(); v != nil {
			return v, false
		}
	}

	//投票
	header := w.chainer.GetChainTailHead(chain)
	choose := header.ID()
	job := info.Job()
	vote := types.ChooseVote{
		Chain:       chain,
		ApplyNumber: job.SysUnitNumber,
		ApplyUHash:  job.SysUnitHash,
		Choose:      choose,
		CreateAt:    time.Now(),
	}

	err := w.backend.Sign(&vote)
	if err != nil {
		w.logger.Error("sign vote failed", "err", err)
		return nil, false
	}
	//存储
	info.ResetMyVote(w.backend.CurrWitness(), &vote)
	return &vote, true
}

func (w *Worker) doMyChoose(chain common.Address, forceGossip bool) {

	vote, isNew := w.getOrCreateMyChoose(chain)
	if vote == nil {
		return
	}

	select {
	case <-w.quite:
		return
	default:
	}

	//如果票数足够
	info, ok := w.dataSet.GetChainData(chain)
	if !ok {
		return
	}
	if info.VoteEnough() {
		return
	}

	w.logger.Trace("do my choosing", "chain", chain)

	if !isNew {
		//检查是否已不匹配
		last := w.chainer.GetChainTailHead(chain).ID()
		if vote.Choose.Equal(last) {
			//再次广播不需要太过频繁，如果票足够则不需要再广播
			if forceGossip && time.Now().Sub(vote.CreateAt) > time.Second*params.OneNumberHaveSecondBySys*2 {

				//向无投票人索要投票
				needGossip, all := []common.Address{}, w.backend.WitnessSet().AllWitness()
				hasVotes := common.AddressList(info.Voters())
				for _, w := range all {
					if !hasVotes.Have(w) {
						needGossip = append(needGossip, w)
					}
				}
				if len(needGossip) == 0 {
					w.logger.Trace("no one need my vote",
						"chain", chain, "all.online", len(all), "voted", len(hasVotes))
					return
				}
				peers := w.gossip(chain, &requestVote{Chain: chain}, needGossip...)
				if peers == 0 {
					vote.CreateAt = time.Time{}
				} else {
					vote.CreateAt = time.Now()
				}
				w.logger.Trace("gossip my vote reply",
					"chain", chain, "choose", vote.Choose, "sends", peers,
					"all.online", len(all), "voted", len(hasVotes),
					"update", vote.CreateAt)
			}
			return
		}
	}
	//广播投票
	peers := w.gossip(chain, &voteMessage{Vote: *vote, Voters: uint8(info.VotesLen())})
	w.logger.Trace("gossip my vote", "chain", chain, "choose", vote.Choose)
	if peers == 0 {
		vote.CreateAt = time.Time{}
	}
}

type voteMessage struct {
	Vote   types.ChooseVote
	Voters uint8
}
type requestVote struct {
	Chain common.Address
}

type MessageCode uint8

const (
	Vote    MessageCode = iota + 1 //发送投票
	ReqVote                        //请求获取对方投票信息
)

type Message struct {
	Code     MessageCode
	Playload []byte
}

func (w *Worker) gossip(chain common.Address, playload interface{}, witness ...common.Address) int {
	var code MessageCode
	switch playload.(type) {
	case *voteMessage:
		code = Vote
	case *requestVote:
		code = ReqVote
	default:
		panic(fmt.Sprintf("can not handle:%T", playload))
	}

	b, err := rlp.EncodeToBytes(&playload)
	if err != nil {
		panic(err)
	}
	msg := Message{
		Code:     code,
		Playload: b,
	}

	if len(witness) == 0 {
		//广播投票
		peers := w.backend.WitnessSet().Gossip(chooseVoteMsgCode, msg)
		w.logger.Trace("gossip vote", "code", code, "chain", chain, "tos", "all", "sends", peers)
		return peers
	}
	peers := w.backend.WitnessSet().GossipTo(chooseVoteMsgCode, msg, witness...)
	w.logger.Trace("gossip vote", "code", code, "chain", chain, "tos", len(witness), "sends", peers)
	return peers
}

func (w *Worker) HandleMsg(witness common.Address, playload []byte) error {
	var msg Message
	if err := rlp.DecodeBytes(playload, &msg); err != nil {
		return err
	}
	switch msg.Code {
	case Vote:
		return w.handleVote(msg.Playload)
	case ReqVote:
		return w.handleReq(witness, msg.Playload)
	default:
		return errors.New("invalid message code")
	}
}

// 处理投票
// 返回错误表示该投票存在问题，如见证人故意发送无效数据等等
func (w *Worker) handleVote(playload []byte) error {
	var msg voteMessage
	if err := rlp.DecodeBytes(playload, &msg); err != nil {
		return err
	}

	//必须是相同投票
	info, ok := w.dataSet.GetChainData(msg.Vote.Chain)
	w.logger.Trace("received vote", "chain", msg.Vote.Chain,
		"votes", msg.Voters, "choose.uhash", msg.Vote.ApplyUHash,
		"exist", ok)
	if !ok {
		return nil
	}

	//简单校验，过滤无效投票
	if info.Job().SysUnitHash != msg.Vote.ApplyUHash {
		return nil
	}
	//检查签名是否正确
	sender, err := types.Sender(w.signer, &msg.Vote)
	if err != nil {
		return errors.Wrap(err, "invalid vote")
	}

	//必须是来自见证人的投票
	if !w.backend.IsWitness(sender) {
		return errors.New("invalid vote:sender is not witness")
	}
	err = w.dataSet.AddVote(sender, &msg.Vote)
	w.logger.Trace("save vote", "chain", msg.Vote.Chain, "witness", sender, "votes", info.VotesLen(), "err", err)

	return nil
}

func (w *Worker) handleReq(witness common.Address, playload []byte) error {
	var msg requestVote
	if err := rlp.DecodeBytes(playload, &msg); err != nil {
		return err
	}

	//给此见证人发送消息
	vote, _ := w.getOrCreateMyChoose(msg.Chain)
	if vote == nil {
		w.logger.Trace("not have vote task", "chain", msg.Chain)
		return nil
	}
	w.gossip(msg.Chain, &voteMessage{Vote: *vote}, witness)
	return nil
}

func (w *Worker) GenerateTxs() (txList []*types.Transaction) {
	logger := w.logger.New("task", "generateTxs")

	var inputs []*sc.ReplaceWitnessExecuteInput

	chains := w.dataSet.Chains()

	logger.Trace("begin", "chains", len(chains))
	defer func() {
		logger.Trace("end", "txs", len(txList))
	}()

	for _, c := range chains {
		info, ok := w.dataSet.GetChainData(c)
		if !ok {
			logger.Trace("missing chain", "chain", c)
			continue
		}
		if !info.VoteEnough() {
			logger.Trace("insufficient votes", "chain", c, "count", info.VotesLen())

			continue
		}

		//数据组装需要一定时间，如果已退出，则立刻结束
		select {
		case <-w.quite:
			return nil
		default:
		}

		vote, signes := info.GetVotes()
		signesList := make([][]byte, 0, len(signes))
		for witness, s := range signes {
			//有可能此刻之前的见证人已经被清退，因此需要关注
			if w.backend.IsWitness(witness) {
				signesList = append(signesList, s)
			}
		}
		if len(signesList) < params.GetChainMinVoting(params.SCAccount) {
			logger.Trace("insufficient votes", "count", len(signesList), "old", len(signes))
			continue
		}

		//如果是创世单元，则无投票记录
		var votes []types.WitenssVote
		if vote.Choose.Height > 0 {
			var err error
			votes, err = core.GetVoteMsg(w.db, vote.Choose.Hash)
			if err != nil {
				logger.Error("get vote failed", "uhash", vote.Choose.Hash, "votes", len(votes), "err", err)
				continue
			}
		}

		if vote.Choose.Height > 0 && len(votes) == 0 {
			logger.Error("get vote failed, votes is empty", "uhash", vote.Choose.Hash)
			continue
		}

		input := sc.ReplaceWitnessExecuteInput{
			Chain:          c,
			ApplyNumber:    vote.ApplyNumber,
			ApplyUHash:     vote.ApplyUHash,
			Choose:         vote.Choose,
			ChooseVoteSign: signesList,
			UnitVoterSigns: votes,
		}
		inputs = append(inputs, &input)
		logger.Trace("push a exec replace witness tx",
			"chain", input.Chain, "applyNumber", input.ApplyNumber, "choose", vote.Choose)
	}

	//再将输入打包成交易
	//因为涉及数据大小，需要拆分成小包
	if len(inputs) == 0 {
		return nil
	}

	batch := 10
	for i := 0; i < len(inputs); i += batch {
		end := i + batch
		if end >= len(inputs) {
			end = len(inputs)
		}
		data := inputs[i:end]

		input, err := sc.MakeReplaceWitnessExecuteInput(data)
		if err != nil {
			logger.Error("pack input data failed", "err", err)
			//不继续
			return nil
		}
		tx := types.NewTransaction(w.backend.CurrWitness(), params.SCAccount, nil, 0, 0, types.DefTxMaxTimeout, input)
		if err := w.backend.Sign(tx); err != nil {
			logger.Error("sign tx failed", "err", err)
			return nil
		}
		logger.Trace("new tx", "txhash", tx.Hash(), "items", len(data))
		txList = append(txList, tx)
	}

	return txList
}
