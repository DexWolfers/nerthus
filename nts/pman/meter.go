package pman

import (
	"gitee.com/nerthus/nerthus/metrics"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"

	rcmetrics "github.com/rcrowley/go-metrics"
)

var (
	propTxnInPacketsMeter    *rcmetrics.NilMeter
	propTxnInTrafficMeter    *rcmetrics.NilMeter
	propTxnOutPacketsMeter   *rcmetrics.NilMeter
	propTxnOutTrafficMeter   *rcmetrics.NilMeter
	propHashInPacketsMeter   *rcmetrics.NilMeter
	propHashInTrafficMeter   *rcmetrics.NilMeter
	propHashOutPacketsMeter  *rcmetrics.NilMeter
	propHashOutTrafficMeter  *rcmetrics.NilMeter
	propBlockInPacketsMeter  *rcmetrics.NilMeter
	propBlockInTrafficMeter  *rcmetrics.NilMeter
	propBlockOutPacketsMeter *rcmetrics.NilMeter
	propBlockOutTrafficMeter *rcmetrics.NilMeter
	propBftInTrafficMeter    *rcmetrics.NilMeter
	propBftOutTrafficMeter   *rcmetrics.NilMeter

	miscInPacketsMeter  *rcmetrics.NilMeter
	miscInTrafficMeter  *rcmetrics.NilMeter
	miscOutPacketsMeter *rcmetrics.NilMeter
	miscOutTrafficMeter *rcmetrics.NilMeter
)

// MeteredMsgReadWriter is a wrapper around a p2p.MsgReadWriter, capable of
// accumulating the above defined metrics based on the data stream contents.
type MeteredMsgReadWriter struct {
	p2p.MsgReadWriter     // Wrapped message stream to meter
	version           int // Protocol version to select correct meters
}

// newMeteredMsgWriter wraps a p2p MsgReadWriter with metering support. If the
// metrics system is disabled, this function returns the original object.
func NewMeteredMsgWriter(rw p2p.MsgReadWriter) p2p.MsgReadWriter {
	warp := NewLimitMsgReadWriter(rw)
	if !metrics.Enabled {
		return warp
	}
	return &MeteredMsgReadWriter{MsgReadWriter: warp}
}

// Init sets the protocol version used by the stream to know which meters to
// increment in case of overlapping message ids between protocol versions.
func (rw *MeteredMsgReadWriter) Init(version int) {
	rw.version = version
}

func (rw *MeteredMsgReadWriter) ReadMsg() (p2p.Msg, error) {
	// Read the message and short circuit in case of an error
	msg, err := rw.MsgReadWriter.ReadMsg()
	if err != nil {
		return msg, err
	}
	// Account for the data traffic
	packets, traffic := miscInPacketsMeter, miscInTrafficMeter
	code := uint8(msg.Code)
	switch {
	case code == protocol.NewUnitMsg:
		packets, traffic = propBlockInPacketsMeter, propBlockInTrafficMeter
	case code == protocol.TxMsg:
		packets, traffic = propTxnInPacketsMeter, propTxnInTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	return msg, err
}

func (rw *MeteredMsgReadWriter) WriteMsg(msg p2p.Msg) error {
	// Account for the data traffic
	packets, traffic := miscOutPacketsMeter, miscOutTrafficMeter
	code := uint8(msg.Code)
	switch {
	case code == protocol.NewUnitMsg:
		packets, traffic = propBlockOutPacketsMeter, propBlockOutTrafficMeter
	case code == protocol.TxMsg:
		packets, traffic = propTxnOutPacketsMeter, propTxnOutTrafficMeter
	case code == protocol.BftMsg:
		packets, traffic = propBftInTrafficMeter, propBftOutTrafficMeter
	}
	packets.Mark(1)
	traffic.Mark(int64(msg.Size))

	// Send the packet to the p2p layer
	return rw.MsgReadWriter.WriteMsg(msg)
}
