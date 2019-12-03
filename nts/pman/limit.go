package pman

import (
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/metrics"

	"gitee.com/nerthus/nerthus/common/bytefmt"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
)

// MeteredMsgReadWriter is a wrapper around a p2p.MsgReadWriter, capable of
// accumulating the above defined metrics based on the data stream contents.
type LimitMsgReadWriter struct {
	p2p.MsgReadWriter // Wrapped message stream to meter
}

// newMeteredMsgWriter wraps a p2p MsgReadWriter with metering support. If the
// metrics system is disabled, this function returns the original object.
func NewLimitMsgReadWriter(rw p2p.MsgReadWriter) p2p.MsgReadWriter {
	ratioInit.Do(initRatio)
	return &LimitMsgReadWriter{MsgReadWriter: rw}
}

func (rw *LimitMsgReadWriter) ReadMsg() (p2p.Msg, error) {
	return rw.MsgReadWriter.ReadMsg()
}

func (rw *LimitMsgReadWriter) WriteMsg(msg p2p.Msg) error {
	if enableLimitCheck {
		trafficOutLimitCheck(msg.Size)

		// Account for the data traffic
		code := uint8(msg.Code)
		switch {
		case code == protocol.TxMsg:
			txTrafficLimit(msg.Size)
		}
	}
	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}

const (
	cfgKeyTxTrafficOutLimit  = "runtime.p2p_tx_traffic_out_pre_second"
	cfgKeySumTrafficOutLimit = "runtime.p2p_traffic_out_limit"
)

var (
	txTrafficOutLimit float64
	enableLimitCheck  bool
	ratioInit         sync.Once
	txOutTrafficMeter metrics.Meter
	txOutTrafficLock  sync.Locker

	sumOutTrafficMeter metrics.Meter
	sumOutTrafficLimit float64
	sumOutTrafficLock  sync.Locker
)

func loadCfg(key string) float64 {
	v := config.MustString(key, "")
	if v == "" || v == "0" {
		return 0
	}
	byteV, err := bytefmt.ToBytes(v)
	if err != nil {
		log.Crit("invalid config value", "key", key, "value", v, "err", err)
	}
	return float64(byteV)
}

func initRatio() {

	//设置交易通道总流量
	initTx := func() {
		limit := loadCfg(cfgKeyTxTrafficOutLimit)
		if limit == 0 {
			return
		}
		enableLimitCheck = true
		txTrafficOutLimit = limit
		txOutTrafficMeter = metrics.NewRegisteredMeterForced("p2p/traffic/out/tx", nil)
		txOutTrafficLock = &sync.Mutex{}
	}
	// 同流量通道
	initSum := func() {
		limit := loadCfg(cfgKeySumTrafficOutLimit)
		if limit == 0 {
			return
		}
		enableLimitCheck = true
		sumOutTrafficLimit = limit
		sumOutTrafficMeter = metrics.NewRegisteredMeterForced("p2p/traffic/out", nil)
		sumOutTrafficLock = &sync.Mutex{}
	}

	initTx()
	initSum()
}

func txTrafficLimit(size uint32) {
	if txOutTrafficMeter == nil {
		return
	}
	txOutTrafficLock.Lock()
	txOutTrafficMeter.Mark(int64(size))
	//如果超过流量限制则
	if txOutTrafficMeter.RateMean() > txTrafficOutLimit {
		time.Sleep(time.Second * 2)
	}
	txOutTrafficLock.Unlock()
}

func trafficOutLimitCheck(size uint32) {
	if sumOutTrafficMeter == nil {
		return
	}
	// 必须采用锁进行检查，因为一旦在等待发送，则不能因为无锁导致批量消息均同时出现恢复，引起消息流。
	sumOutTrafficLock.Lock()
	sumOutTrafficMeter.Mark(int64(size))
	//如果超过流量限制则
	if sumOutTrafficMeter.RateMean() > sumOutTrafficLimit {
		log.Debug("trafficOutLimit sleep", "limit", sumOutTrafficLimit, "current", sumOutTrafficMeter.RateMean())
		time.Sleep(time.Second * 2)
	}
	sumOutTrafficLock.Unlock()
}
