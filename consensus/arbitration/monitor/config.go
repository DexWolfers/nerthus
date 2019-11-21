package monitor

import "gitee.com/nerthus/nerthus/params"

type Config struct {
	ErrorInterval uint64 // 进入异常状态间隔 s
	StopInterval  uint64 //进入停止状态间隔 s
	CheckInterval uint64 // 检查间隔 	s
}

func NewDefaultConfig() Config {
	return Config{
		ErrorInterval: uint64(params.SCDeadArbitrationInterval * 0.8),
		StopInterval:  uint64(params.SCDeadArbitrationInterval),
		CheckInterval: 60,
	}
	//return Config{
	//	ErrorInterval: 40,
	//	StopInterval:  60,
	//	CheckInterval: 30,
	//}
}
