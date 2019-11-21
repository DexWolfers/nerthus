package clean

import (
	configpkg "gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/params"
	"math"
)

type Config struct {
	AutoClearOn       bool   //开关
	Period            uint64 //一个周期包含高度
	IntervalPeriod    uint64 //用户链保留的周期数
	SysIntervalPeriod uint64 //系统链保留的周期
}

func NewConfig() Config {
	config := defaultConfig()
	config.AutoClearOn = configpkg.MustBool("runtime.statedb_clean_switch", config.AutoClearOn)
	p := configpkg.MustInt("runtime.statedb_clean_period", int(config.Period))
	if p > 0 {
		config.Period = uint64(p)
	}
	iv := configpkg.MustInt("runtime.statedb_clean_interval", int(config.IntervalPeriod))
	if iv > 0 {
		config.IntervalPeriod = uint64(iv)
	}
	// 系统链保留最大取现周期的单元
	config.SysIntervalPeriod = uint64(math.Ceil(float64(params.ConfigParamsBillingPeriod*params.MaxSettlementPeriod) / float64(config.Period)))
	return config
}

func defaultConfig() Config {
	cfg := Config{
		AutoClearOn:    true,
		Period:         100,
		IntervalPeriod: 1,
	}
	return cfg
}
