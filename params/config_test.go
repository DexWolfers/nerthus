package params

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	v := viper.New()
	v.SetConfigType("YAML")

	var configYaml = ` 
chainId: 111111
ucWitness: 1112
ucMinVotings: 1113
ucWitnessCap: 1119
ucWitnessMargin: 1117
scWitness: 1110
scMinVotings: 1114
minFreePoW: 18 
`

	err := v.ReadConfig(bytes.NewBuffer([]byte(configYaml)))
	require.NoError(t, err)

	cfg, err := ParseChainConfig(v)
	require.NoError(t, err)

	keys := []string{"ucWitness", "ucMinVotings", "ucWitnessCap", "ucWitnessMargin", "scWitness", "scMinVotings", "minFreePoW"}
	for _, k := range keys {
		want := v.GetInt(k)
		assert.Equal(t, want, int(cfg.Get(k)), "should be get the key value %s", k)
	}

}

func TestLoadd(t *testing.T) {

	var dataYaml = `
importKeys:
  -
    address: nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c
    keyHex: f5c23dcd0ca8a284de196564f8fba635e0f0c48756d12b1f67206f06cedfc3be
    keyContent: "{'address':'nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c','crypto':{'cipher':'aes-128-ctr','ciphertext':'43432655130bb460254e87795fb02665093c6f927f39bd92977d6edbf8b8e487','cipherparams':{'iv':'26932ffa25bd8f84282ac93b6cb49170'},'kdf':'scrypt','kdfparams':{'dklen':32,'n':262144,'p':1,'r':8,'salt':'20552922023d9d5d54964ec59aea18802cb584606a0644d6c74a897aad8ce2fc'},'mac':'8a6d86bbdab69cd370a325435325b431919bd1305410aeb73a1528fa9dcbdcd9'},'id':'3739da3b-3fa1-4fc7-a757-348dbc4d5c0f','version':3}"
`
	v := viper.New()
	v.SetConfigType("YAML")
	err := v.ReadConfig(bytes.NewBuffer([]byte(dataYaml)))
	require.NoError(t, err)

	var list []devAccountInfo
	err = v.UnmarshalKey("importKeys", &list)
	require.NoError(t, err)
	require.Len(t, list, 1, "should contains one item")

	assert.Equal(t, "nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c", list[0].Address)
	assert.Equal(t, "f5c23dcd0ca8a284de196564f8fba635e0f0c48756d12b1f67206f06cedfc3be", list[0].KeyHex)
	assert.NotEmpty(t, list[0].KeyContent)

	// test config
	testChainconfig := &ChainConfig{
		ChainId: big.NewInt(66),
		consensusConfig: map[string]uint64{
			ConfigParamsUCWitnessCap:   400,
			ConfigParamsCouncilCap:     33,
			ConfigParamsLowestGasPrice: 1,
			ConfigParamsMinFreePoW:     20,
			//ConfigParamsTimeApply:          30,
			//ConfigParamsTimeFinalize: 200,
		},
		// 提案参数
		Proposal: map[string]bool{
			ConfigParamsUCWitnessCap: true,
			ConfigParamsCouncilCap:   true,
			ConfigParamsMinFreePoW:   true,
			//ConfigParamsTimeApply:          true,
			//ConfigParamsTimeFinalize:      true,
		},
	}

	assert.Equal(t, uint64(33), testChainconfig.Get(ConfigParamsCouncilCap))
	//assert.Equal(t, uint64(200), testChainconfig.Get(ConfigParamsTimeFinalize))
	cfg := testChainconfig.GetConfigs()
	assert.Equal(t, uint64(33), cfg[ConfigParamsCouncilCap])
	//assert.Equal(t, uint64(200), cfg[ConfigParamsTimeFinalize])
}
