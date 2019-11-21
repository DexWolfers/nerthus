package txpool

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

var keyLen = len(encodeKey(ExecMsg{}))

func getChainPrefix(chain common.Address) []byte {
	return append(dataPrekey, chain.Bytes()...)
}

func encodeKey(msg ExecMsg) []byte {
	return append(getChainPrefix(msg.Chain), append(msg.TxHash.Bytes(), byte(msg.Action))...)
}

func decodeKey(key []byte) ExecMsg {
	if len(key) != keyLen {
		return ExecMsg{}
	}
	return ExecMsg{
		Chain:  common.BytesToAddress(key[len(dataPrekey) : len(dataPrekey)+common.AddressLength]),
		TxHash: common.BytesToHash(key[len(dataPrekey)+common.AddressLength : len(dataPrekey)+common.AddressLength+common.HashLength]),
		Action: types.TxAction(uint8(key[len(key)-1])),
	}
}
