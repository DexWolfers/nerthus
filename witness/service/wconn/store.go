package wconn

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	nodeInfoPrefix    = []byte("currentGroupWNode")
	nodeConnectPrefix = []byte("witness_connect_info")
)

func getNodeKey(witness common.Address) []byte {
	return append(nodeInfoPrefix, witness.Bytes()...)
}
func getNodeConnectKey(witness common.Address) []byte {
	return append(nodeConnectPrefix, witness.Bytes()...)
}

// 获取本地存储的见证人网络连接信息
func GetAllWitnessNodeInfo(db ntsdb.Database) (nodes []*WitnessNode, err error) {
	nodes = make([]*WitnessNode, 0, params.ConfigParamsUCWitness)

	db.Iterator(nodeInfoPrefix, func(key, value []byte) bool {
		node := new(WitnessNode)
		err = rlp.DecodeBytes(value, &node)
		if err != nil {
			return false
		}
		nodes = append(nodes, node)
		return true
	})
	return
}

// 存储更新指定见证人信息
func WriteWintessNodeInfo(db ntsdb.Database, node WitnessNode) {
	b, err := rlp.EncodeToBytes(&node)
	if err != nil {
		panic(err)
	}
	db.Put(getNodeKey(node.Witness), b)
}
func GetWintessNodeInfo(db ntsdb.Database, witness common.Address) *WitnessNode {
	b, _ := db.Get(getNodeKey(witness))
	if len(b) == 0 {
		return nil
	}
	var node WitnessNode
	err := rlp.DecodeBytes(b, &node)
	if err != nil {
		panic(err)
	}
	return &node
}
func DelWitnessNodeInfo(db ntsdb.Database, witness common.Address) {
	db.Delete(getNodeKey(witness))
}

// 存储见证人网络连接信息
func WriteWitnessConnectInfo(db ntsdb.Database, witness common.Address, msg *WitnessConnectMsg) {
	b, err := rlp.EncodeToBytes(msg)
	if err != nil {
		panic(err)
	}
	db.Put(getNodeConnectKey(witness), b)
}
func DelWitnessConnectInfo(db ntsdb.Database, witness common.Address) {
	db.Delete(getNodeConnectKey(witness))
}

// 获取该见证人所对应的网络连接信息
func GetWitnessConnectInfo(db ntsdb.Database, witness common.Address) *WitnessConnectMsg {
	b, _ := db.Get(getNodeConnectKey(witness))
	if len(b) == 0 {
		return nil
	}
	info := new(WitnessConnectMsg)
	if err := rlp.DecodeBytes(b, &info); err != nil {
		panic(err)
	}
	return info
}
