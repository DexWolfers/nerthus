
# 更换见证人系统见证人指定最后合法单元 

包名rwit是replace Witness的简称。 

## 需求

为解决更换链见证人时，新旧两批见证人需要指定一个合法交接高度。防止出现新旧见证人在相同高度出单元，而共识无法校验两种谁才合法。

按当前规则，先到先合法，这使得不同节点的视图不一致，出现混乱和双花。

因此，需要由系统见证人在链申请更换见证人时，指定当前见证人最后的合法高度。
相对于为新旧两批见证人换分了分界线。


## 实现方案

用户在申请见证人时，分配见证人但不要求新见证人立即干活，而是等待系统见证人指定合法位置。


1. 监听链状态事件（正常、更换见证人中）。
    1. 如果是更换中，则记录到DB，并推送事件。
    2. 如果是正常，则删除 DB 记录
1. 事件订阅链工作事件，如果有则进行投票。
    注意：为了提高有效率，不需要在接收到事件时立即投票，而是需要等待3个单元时间，争取让每个人的视图一致。
1. 每次提交执行合法位置，为了提高效率。允许按10个地址一批次提交。 
1. 为防止流量攻击，对所有链进行排队处理。
1. 投票收集完成后，本地存储投票集。
1. 由单元提案者提交已完成投票部分。    

## 投票细节

不使用拜占庭投票，而是只忠于自己的视角。
因此在收集其他见证人投票时，只保留和本人相同内容的投票，忽略其他。


1. 3个单元后，开始投票（chain+ lastHeader）
2. 收集投票到集合 S，只有投票 vote.LastHeader = myVote.LastHeader 时才存储内存。
3. 一旦发现自己接收到该链的新高度单元，统一视为有效。立即丢弃当前投票以及集合 S。
4. 重新开始投票，重复步骤1。
5. 当票数超过8票时，推入待打包队列 VotedQueue。
6. 当轮到自己充当将军时，将 VotedQueue 中投票组成成交易，推入交易处理池。
7. 有可能其他人已先行处理，此时可清理对于此链的处理。
8. 为了防止程序例外终止而丢失操作情况，需要实时将 Pending 中的链记录到 DB 中，便于重启后快速开始。
9. 在首次加载时，需要检查数据是否过时，否则引起无法投票情况。