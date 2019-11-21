// Author: @ysqi

package core

import (
	"fmt"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/params"

	"github.com/spf13/viper"
)

// NOTE(ysqi): current config sets with develop address.

// main network allocation
// TODO(ysqi): update alloc for main net
var (
	mainnetWMGAcc    = devWMGAcc
	mainnetAllocData = devAllocData
)

// test network allocation
var (
	testnetWMGAcc    = devWMGAcc
	testnetAllocData = devAllocData
)

// development allocation
var (
	devWMGAcc    = common.GenerateContractAddress(common.StringToAddress("genesis"), 0x6060, 1)
	devAllocData = `
[ 
   { "address": "nts1e5r8jrtwhpapx6tjs5rnqu4qm65pmyc25wwuhd", "Balance": 500000000000000000},
   { "address": "nts1z74sa7qs7raxmtt3qhq8yvwen2me5tlgwysl3f", "Balance": 500000000000000000},
   { "address": "nts1keyregdxra3rehsa4eqk385gfj4t2chpvjnza6", "Balance": 500000000000000000},
   { "address": "nts1wlddd64hgk9r9nn6xf7wzzqyd2uvm22de3s5nm", "Balance": 500000000000000000},
   { "address": "nts1j29sk5pzh42eguculnrjrnefpuqf3nccssrz5m", "Balance": 500000000000000000},
   { "address": "nts1gg02ftz0nwv9gz889my7gwxcykvfhy6r6gdffx", "Balance": 500000000000000000},
   { "address": "nts1npj09rj6eja3m8chwr34c3y8453fkxxhvhk5qc", "Balance": 500000000000000000},
   { "address": "nts1hzemfg5x88hzrdh3kqefxtzptyy8hraex0w6lc", "Balance": 500000000000000000},
   { "address": "nts1cdh2se8myzhsd7nwrxn88r54rm6vd58d9ynhqg", "Balance": 500000000000000000},
   { "address": "nts1fdpn0dy7r595fd4zs4qpwp853mldk3tthmejq0", "Balance": 500000000000000000},
   { "address": "nts1c6m9z0mf8sw3yv5lamax6jqm9kwr932sqyy3sk", "Balance": 500000000000000000},
   { "address": "nts13nfdv0t7ca5qnf6mfs5m4ff3sgaashj2cngmvz", "Balance": 500000000000000000},
   { "address": "nts1fuvz9fmzr5j5gdnzntvv8mtct6ws648fz32v6z", "Balance": 500000000000000000},
   { "address": "nts1f6p53jcs60840s5rtkznvuay7xnv5wmrrsygnk", "Balance": 500000000000000000},
   { "address": "nts19aqvh78y5zs7dz22rd462tujefq2ce57ca0cc3", "Balance": 500000000000000000},
   { "address": "nts122ylt6nmj0hjan894qdlswqgdwupxpw5sgr0sq", "Balance": 500000000000000000},
   { "address": "nts1cj4razp8ej6wsjfrnrw0rshhalvqggdrkknljv", "Balance": 500000000000000000},
   { "address": "nts15y84ug6f90sjdwq4jj0e8cfnarr3cqua9enaz6", "Balance": 500000000000000000},
   { "address": "nts1r3ga242u272jg7krmmz3348ak924xjhrlqsxny", "Balance": 500000000000000000},
   { "address": "nts1h4mmcwjntrtly9q980f2nkjc7aawcpu9xmv456", "Balance": 500000000000000000},
   { "address": "nts10u4wvj7yvnlr77qrq5srlw7zpfdy8sp7xfcr0p", "Balance": 500000000000000000},
   { "address": "nts1ah0x8zus88n36d0jnyetexu8lwzxp3v34l5p20", "Balance": 500000000000000000},
   { "address": "nts1rrfd95t7yx3q20u9d73h58f2kdfdjas2h7sjpr", "Balance": 500000000000000000}, 
   { "address": "nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql", "Balance": 500000000000000000},
   { "address": "nts1wrk344fy4hncw2w92cn7yszspav3xj2sn253tx", "Balance": 500000000000000000},
   { "address": "nts1aqlce3yp04nd6r36pfn794ydtx0a7s7ruqkkzd", "Balance": 500000000000000000},
   { "address": "nts13j7sc0yqmyhhne5d3jnecqwr7u4746ahq64txx", "Balance": 500000000000000000},
   { "address": "nts1za2mha7cutrjjgrtw6mup6pkc2qry7rh4xtx43", "Balance": 500000000000000000},
   { "address": "nts16p0x9arx4z5af244ykn49wth277r4fydvd78n7", "Balance": 500000000000000000},
   { "address": "nts1l9u8lu5qncue99g70tgtfdzld5lswf6vvypfvx", "Balance": 500000000000000000},
   { "address": "nts1acu3jx5c4xva04893vffwdf9582zz48p7zf3l5", "Balance": 500000000000000000},
   { "address": "nts14wapa6pad9f57d725tt3s6wldt6rkskcccxtzg", "Balance": 500000000000000000},
   { "address": "nts1aq2auh2ete2ta8vzw6zq36lgqwjhj3gtuv8pju", "Balance": 500000000000000000}, 
   { "address": "nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c", "Balance": 500000000000000000},
   { "address": "nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql", "Balance": 500000000000000000}, 
   { "address": "nts1arjpwgcxwcnur99wpeun07etejw9r6j5awyqzg", "Balance": 500000000000000000},
   { "address": "nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u", "Balance": 500000000000000000},
   { "address": "nts1a8a9k9pnd532aszw3cvmqmu99qfwhzzx4jjpqr", "Balance": 500000000000000000},
   { "address": "nts1e5r8jrtwhpapx6tjs5rnqu4qm65pmyc25wwuhd", "Balance": 500000000000000000},
   { "address": "nts1z74sa7qs7raxmtt3qhq8yvwen2me5tlgwysl3f", "Balance": 500000000000000000},
   { "address": "nts1j29sk5pzh42eguculnrjrnefpuqf3nccssrz5m", "Balance": 500000000000000000},
   { "address": "nts1gg02ftz0nwv9gz889my7gwxcykvfhy6r6gdffx", "Balance": 500000000000000000},
   { "address": "nts1hzemfg5x88hzrdh3kqefxtzptyy8hraex0w6lc", "Balance": 500000000000000000},
   { "address": "nts1npj09rj6eja3m8chwr34c3y8453fkxxhvhk5qc", "Balance": 500000000000000000},
   { "address": "nts1keyregdxra3rehsa4eqk385gfj4t2chpvjnza6", "Balance": 500000000000000000},
   { "address": "nts154ffjespv0wdeela09398qc90entwf6z858jyw", "Balance": 500000000000000000},
   { "address": "nts1wlddd64hgk9r9nn6xf7wzzqyd2uvm22de3s5nm", "Balance": 500000000000000000}]
`
)

var (
	mainnetDefaultWitness = devDefaultWitness
	testnetDefaultWitness = devDefaultWitness
	devDefaultWitness     = []string{
		"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql",
		"nts1wrk344fy4hncw2w92cn7yszspav3xj2sn253tx",
		"nts1aqlce3yp04nd6r36pfn794ydtx0a7s7ruqkkzd",
		"nts13j7sc0yqmyhhne5d3jnecqwr7u4746ahq64txx",
		"nts1za2mha7cutrjjgrtw6mup6pkc2qry7rh4xtx43",
		"nts16p0x9arx4z5af244ykn49wth277r4fydvd78n7",
		"nts1l9u8lu5qncue99g70tgtfdzld5lswf6vvypfvx",
		"nts1acu3jx5c4xva04893vffwdf9582zz48p7zf3l5",
		"nts14wapa6pad9f57d725tt3s6wldt6rkskcccxtzg",
		"nts1aq2auh2ete2ta8vzw6zq36lgqwjhj3gtuv8pju",

		"nts10u4wvj7yvnlr77qrq5srlw7zpfdy8sp7xfcr0p",
		"nts1rrfd95t7yx3q20u9d73h58f2kdfdjas2h7sjpr",
	}

	mainnetSystemWitness = devSystemWitness
	testnetSystemWitness = devSystemWitness
	devSystemWitness     = []string{
		"nts1e5r8jrtwhpapx6tjs5rnqu4qm65pmyc25wwuhd",
		"nts1z74sa7qs7raxmtt3qhq8yvwen2me5tlgwysl3f",
		"nts1keyregdxra3rehsa4eqk385gfj4t2chpvjnza6",
		"nts1wlddd64hgk9r9nn6xf7wzzqyd2uvm22de3s5nm",
		"nts1j29sk5pzh42eguculnrjrnefpuqf3nccssrz5m",
		"nts1gg02ftz0nwv9gz889my7gwxcykvfhy6r6gdffx",
		"nts1npj09rj6eja3m8chwr34c3y8453fkxxhvhk5qc",
		"nts1hzemfg5x88hzrdh3kqefxtzptyy8hraex0w6lc",
		"nts1cdh2se8myzhsd7nwrxn88r54rm6vd58d9ynhqg",
		"nts1fdpn0dy7r595fd4zs4qpwp853mldk3tthmejq0",

		"nts1c6m9z0mf8sw3yv5lamax6jqm9kwr932sqyy3sk",
		"nts13nfdv0t7ca5qnf6mfs5m4ff3sgaashj2cngmvz",
		"nts1fuvz9fmzr5j5gdnzntvv8mtct6ws648fz32v6z",
		"nts1f6p53jcs60840s5rtkznvuay7xnv5wmrrsygnk",
		"nts19aqvh78y5zs7dz22rd462tujefq2ce57ca0cc3",
		"nts122ylt6nmj0hjan894qdlswqgdwupxpw5sgr0sq",
		"nts1cj4razp8ej6wsjfrnrw0rshhalvqggdrkknljv",
		"nts15y84ug6f90sjdwq4jj0e8cfnarr3cqua9enaz6",
		"nts1r3ga242u272jg7krmmz3348ak924xjhrlqsxny",
		"nts1h4mmcwjntrtly9q980f2nkjc7aawcpu9xmv456",

		"nts1ah0x8zus88n36d0jnyetexu8lwzxp3v34l5p20",
	}
)

var (
	mainnetDefaultCouncilMembers = devDefaultCouncilMembers
	testnetDefaultCouncilMembers = devDefaultCouncilMembers
	devDefaultCouncilMembers     = []common.Address{
		common.ForceDecodeAddress("nts1arjpwgcxwcnur99wpeun07etejw9r6j5awyqzg"),
		common.ForceDecodeAddress("nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u"),
		common.ForceDecodeAddress("nts154ffjespv0wdeela09398qc90entwf6z858jyw"),
	}
)

func LoadGenesisConfig() (g *Genesis, err error) {
	g = &Genesis{
		Alloc:     make(GenesisAlloc),
		Timestamp: uint64(params.GenesisUTCTime.UnixNano()),
	}
	// 创世账户
	var accounts []struct {
		Address string
		Balance string
	}
	if err = viper.UnmarshalKey("genesis.accounts", &accounts); err != nil {
		return
	}
	for _, a := range accounts {
		addr, err := common.DecodeAddress(a.Address)
		if err != nil {
			return nil, err
		}
		balance, ok := math.ParseBig256(a.Balance)
		if !ok {
			return nil, fmt.Errorf("invalid balance %q", a.Balance)
		}
		g.Alloc[addr] = GenesisAccount{
			Balance: balance,
		}
	}

	// 配置
	cfg, err := params.ParseChainConfig(viper.Sub("chain"))
	if err != nil {
		return
	}

	g.Config = &cfg

	return
}

type yamlList struct {
	Address string
	KeyHex  string
}

// DefaultSystemWitness 默认系统见证人
func DefaultSystemWitness() []List {
	//values := viper.GetStringSlice("genesis.systemChain.witness")
	//return toAddressList(values)
	return toYamlList("genesis.systemChain.witness")
}

// DefaultUserWitness 默认用户链见证人
func DefaultUserWitness() []List {
	return toYamlList("genesis.normalChain.witness")
}

// DefaultCouncilMember 默认理事成员
func DefaultCouncilMember() []common.Address {
	values := viper.GetStringSlice("genesis.council.member")
	return toAddressList(values)
}

func toAddressList(items []string) []common.Address {
	lib := make([]common.Address, len(items))
	for i, v := range items {
		lib[i] = common.ForceDecodeAddress(v)
	}
	return lib
}

type List = struct {
	Address common.Address
	PubKey  []byte
}

func toYamlList(key string) []List {
	yamlList := make([]yamlList, 0)
	viper.UnmarshalKey(key, &yamlList)
	var result []List
	for _, v := range yamlList {
		addr := common.ForceDecodeAddress(v.Address)
		result = append(result, List{addr, common.Hex2Bytes(v.KeyHex)})
	}
	return result
}
