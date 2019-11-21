package sc

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
	return key, addr
}

func TestINfo(t *testing.T) {
	input := common.FromHex("0xfd4322ea00000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000acaf90ac7f901e594b129c093b7ce58331fc41868f010926af140598d01a07465663b1df4be7f41cdbe0c2fd2063a0e997970c73968c132ee54051e350987a07d561d6443de5731b21124a771183bba2b4b50d228dfd8bae7585a57b4280fa4820e01941755bbf7d8e2c729206b76b7c0e836c280327877a043dd6fdb6faa1c6cd6fe636f93843870ae5d2fd62d3f02b23179aabe97d0f1e2a005bb4c126dc98fbfac787cede03180140611afe7c9264a7aaa14257fa1ab81d0a07f31b2eec6799942dcbfffa709fd859ae47157c7802c07ae0e20898a495e46afb90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008402625a00808815a4effa0d8b6975f908dcf9046bf901e594b129c093b7ce58331fc41868f010926af140598d01a07465663b1df4be7f41cdbe0c2fd2063a0e997970c73968c132ee54051e350987a07d561d6443de5731b21124a771183bba2b4b50d228dfd8bae7585a57b4280fa4820e01941755bbf7d8e2c729206b76b7c0e836c280327877a043dd6fdb6faa1c6cd6fe636f93843870ae5d2fd62d3f02b23179aabe97d0f1e2a005bb4c126dc98fbfac787cede03180140611afe7c9264a7aaa14257fa1ab81d0a07f31b2eec6799942dcbfffa709fd859ae47157c7802c07ae0e20898a495e46afb90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008402625a00808815a4df09884a9e88f90280f84e89030000000000000000b8423439145805a74c4be6eee0f773bcc29d942523ca7535b8e6520212316b32848804169f1f7a57bafbb027c850772644ae177959ee7fae7c1bc09b776cdec1911b1197f84e89030000000000000000b8429ddd1b53e47a5cd6d31c8379ca801a1a97b66ae54deaf3b14f4c49da38927ba92fe06481bbfa1fa05ba7749d1be4942e25cd8f2a8bf1c7f70c8c7637587c3b8e1197f84e89030000000000000000b8429f1a46db6a303dee1ae143af4f52e293c80e1dba3f593095ba654c3658203dda2caffdbbef463da7666cee866f4bb99dd476e838cc4a2d0e91a877b24994885f1197f84e89030000000000000000b842a4dfee0bc9019f47c2b2790ae2a52e5df8c495333dc53e06278cdc73ed93105b43fe6d79c2e7c65425fa0264de5707a12bc9690de311de1e5946fd886d73855f1198f84e89030000000000000000b842b451ad88788db51303d7fa082ef8fd53fd7227fa1b6f79474a5586d0adcf756778bf9962ddd95ebebc6dae171f297fa3ce2f1675344d0a92fddf8c1c081f30a01198f84e89030000000000000000b842de3bcc6b382b5e78c393008288f5d8b1ecfa738c82ab9b03044639c7234014ac2a9d0f3566c9b625e2796f5cabb12586fc25debd094915ebf276c86cdbd870411197f84e89030000000000000000b842def4869160ad2b5bb375cf5708731be18746c9478409eab32299e4a748a0ba3e48f962fd7e0e06614072439d9fac26b6f4696036ed32aaf122286b543a08ce3f1198f84e89030000000000000000b842f48569678381500f6e8255a8592977599cb2799a7c66c6548b900c5ebbb2b0d45f62afc7ebb12d16c74577ca2b344ab5b1761a66d43a71e6d684fb5f2ff438551197f9046bf901e594b129c093b7ce58331fc41868f010926af140598d01a07465663b1df4be7f41cdbe0c2fd2063a0e997970c73968c132ee54051e350987a07d561d6443de5731b21124a771183bba2b4b50d228dfd8bae7585a57b4280fa4820e01941755bbf7d8e2c729206b76b7c0e836c280327877a043dd6fdb6faa1c6cd6fe636f93843870ae5d2fd62d3f02b23179aabe97d0f1e2a005bb4c126dc98fbfac787cede03180140611afe7c9264a7aaa14257fa1ab81d0a07f31b2eec6799942dcbfffa709fd859ae47157c7802c07ae0e20898a495e46afb90100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008402625a00808815a4effa0d8b6975f90280f84e89030000000000000000b842015dce2e9d8cd947d90842e03dbae20e4dfa022d21168d42e8000ec3b497f0d550fff6e8ae134c2dacabf53a15947dc118213d7d27dd0c17d52140acbca657b91197f84e89030000000000000000b84235891ee5c38abe574128954c9cb9f3eb29f48e7cd4898cc9d5c3d5b536db9b6b1521f9c0a9cb8c161f615255b45da47a382ce17005f4caeb40095f7937d7241d1197f84e89030000000000000000b8426402ae00b92d09013b355b90e7f13688526e6ccf325e761f28fe5b85a3050e6e75ea3a34b65275b544de3e30b8a31956a2988f26a55ec6b4153ed9371e9044681197f84e89030000000000000000b84276a1fefa03065f63f90437097ae945effb48c1de76b69a4e41e673b3ccee56550640d6b419a9fa5d0701d86c5c08a627558338c4490797f9c4a0e5fb0b7be6171197f84e89030000000000000000b8428cc89013e15444873af724068012e173b5c7b5cd9b83d5437d2b4228009542db3438af020b7548361f8c8bdb6b77a306774e991a276d95b70bbc10c648fd60e81197f84e89030000000000000000b8429166426290c2e5110215d7d0e70a6ae65befce7f299fc357586cb0681f8e9e8d2708aded23cb637b79d54e1a9e71ac53806ada069077a81f65ff2c29f0d157121197f84e89030000000000000000b8429242843d03e52482b66f3368b6beefcd78013bc7e860ef2691fd3875aa4492bf09fb10b5c647e86211a0e358b0b8e29adeae53705c895c0dff95a5c7fe8eb7a71198f84e89030000000000000000b842cf87dcd2ea8b5fa8dd77e1a3e719eee3c5861804a179198d8aa6298f5de0db33203463f12f3813ff581a2ca029a5c6ffafb8bda060727c4abf30233dbcc015e2119800000000000000000000000000000000000000000000")
	got, err := UnPackReportDishonestInput(input)
	require.NoError(t, err)

	signer := types.NewSigner(big.NewInt(2234))
	for _, p := range got.Proof {
		t.Log(p.Header.MC, p.Header.Number, time.Unix(0, int64(p.Header.Timestamp)), p.Header.Hash().Hex())
		for i := 0; i < len(p.Votes); i++ {
			msg := p.Votes[i].ToVote(p.Header.Hash())

			badSender, err := types.Sender(signer, &msg)
			if err != nil {
				require.NoError(t, err)

			}
			t.Log(badSender)
		}
	}

}

func TestUnPackReportDishonestInput(t *testing.T) {
	var r ReportWitnessDishonest
	r.Proof = append(r.Proof, types.AffectedUnit{
		Header: types.Header{MC: common.StringToAddress("testChain"), Number: 100},
		Votes: []types.WitenssVote{
			{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set(make([]byte, 50))},
		},
	})
	r.Proof = append(r.Proof, types.AffectedUnit{
		Header: types.Header{MC: common.StringToAddress("testChain2"), Number: 2000},
		Votes: []types.WitenssVote{
			{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set(make([]byte, 25))},
		},
	})

	input, err := CreateReportDishonestTxInput(r.Proof[0].Header, r.Proof)
	require.NoError(t, err)
	got, err := UnPackReportDishonestInput(input)
	require.NoError(t, err)
	require.Len(t, got.Proof, len(r.Proof))

	for i := 0; i < len(r.Proof); i++ {
		require.Equal(t, r.Proof[i].Header, got.Proof[i].Header, "at index %d", i)
		require.Equal(t, r.Proof[i].Votes, got.Proof[i].Votes, "at index %d", i)
	}
}

func TestPackReportDishonestOutputRLP(t *testing.T) {
	// 检查 abi 编码
	cases := []ReportDishonestOutput{
		{},
		{UnitHash: common.StringToHash("unit"), Number: math.MaxUint64, Chain: common.StringToAddress("chain addr")},
	}
	for _, c := range cases {
		//编码
		output, err := abiObj.Methods["ReportDishonest"].Outputs.Pack(c.UnitHash, c.Chain, c.Number)
		assert.NoError(t, err)

		// 再校验解码
		got, err := PackReportDishonestOutput(output)
		assert.NoError(t, err)
		assert.Equal(t, c, got)
	}
}

func TestWriteChainArbitration(t *testing.T) {
	db := newTestState()

	chooseList := []types.UnitID{
		{
			ChainID: common.StringToAddress("A"),
			Height:  100,
			Hash:    common.StringToHash("unit1"),
		},
		{
			ChainID: common.StringToAddress("A"),
			Height:  101,
			Hash:    common.StringToHash("unit2"),
		},
		{
			ChainID: common.StringToAddress("B"),
			Height:  100,
			Hash:    common.StringToHash("unit3"),
		},
	}

	//t.Run("get", func(t *testing.T) {
	//
	//	number := GetChainArbitrationNumber(db, choose1.ChainID)
	//	require.Zero(t, choose1.Height, number, "仲裁无记录")
	//
	//})

	votes := []common.Address{
		common.StringToAddress("voter1"),
		common.StringToAddress("voter3"),
		common.StringToAddress("voter4"),
		common.StringToAddress("voter5"),
		common.StringToAddress("voter6"),
		common.StringToAddress("voter7"),
	}

	for _, choose := range chooseList {
		for i, v := range votes {
			c, err := writeChainArbitration(db, choose, v)
			require.NoError(t, err, "应该成功写入")
			require.Equal(t, uint64(i+1), c, "投票数据应该是不断累积")

			//允许重复
			c, err = writeChainArbitration(db, choose, v)
			require.NoError(t, err, "应该成功写入")
			require.Equal(t, uint64(i+1), c, "投票数据应该是不断累积")
		}
	}

}

func TestWriteArbitrationNumberFlag(t *testing.T) {
	db := newTestState()

	cases := []struct {
		Chain  common.Address
		Number uint64
		Want   uint64
	}{
		{common.StringToAddress("A"), 100, 100},
		{common.StringToAddress("A"), 101, 100},
		{common.StringToAddress("A"), 102, 100},
		{common.StringToAddress("A"), 99, 99},
		{common.StringToAddress("A"), 99, 99},

		{common.StringToAddress("B"), 100, 100},
		{common.StringToAddress("B"), 88, 88},
	}
	for _, c := range cases {
		writeArbitrationNumberFlag(db, c.Chain, c.Number)

		got := GetChainArbitrationNumber(db, c.Chain)
		require.Equal(t, c.Want, got, "获取链的仲裁位置必须是最小高度")
	}

	// 测试清理结果
	t.Run("clear", func(t *testing.T) {
		cases := []struct {
			Chain  common.Address
			Number uint64
			Want   uint64 //清理后的结果
		}{
			{common.StringToAddress("A"), 100, 99},
			{common.StringToAddress("A"), 101, 99},
			{common.StringToAddress("A"), 102, 99},
			{common.StringToAddress("A"), 99, 0},
			{common.StringToAddress("A"), 99, 0},

			{common.StringToAddress("B"), 100, 88},
			{common.StringToAddress("B"), 88, 0},
		}
		//先存放
		for _, c := range cases {
			writeArbitrationNumberFlag(db, c.Chain, c.Number)
		}

		//依次删除
		for _, c := range cases {
			clearArbitrationNumberFlag(db, c.Chain, c.Number)

			got := GetChainArbitrationNumber(db, c.Chain)
			require.Equal(t, c.Want, got, "应该继续保留最小高度，直到清理完毕")

		}
	})

}
