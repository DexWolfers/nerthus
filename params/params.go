package params

// 定义全局的常量数据

var (
	PrefixSystemWitnessRank = []byte("system_witness_rank")
	PrefixMember            = []byte("mm")
)

// 每组见证人数量
const RankGroupCount = 11

const (
	SettlePeriodTime = 7 * 24 * 60 * 60 // 清算期次时间间隔(秒)
	//SettlePeriodTime = 300 // 测试用
	//SettlePeriodTime = 3 // 测试用
)
