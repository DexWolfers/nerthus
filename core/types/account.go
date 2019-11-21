package types

//go:generate enumer -type=AccountRole -json -yaml -text  -transform=snake -trimprefix=Acc

// 账户角色
type AccountRole int8

const (
	AccNormal              AccountRole = iota //普通账户
	AccMainSystemWitness                      //主系统见证人
	AccSecondSystemWitness                    //备选系统见证人
	AccUserWitness                            //用户见证人
	AccCouncil                                //理事
	AccContract                               //合约
)
