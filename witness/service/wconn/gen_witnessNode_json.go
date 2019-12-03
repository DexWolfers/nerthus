// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package wconn

import (
	"encoding/json"
	"errors"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

var _ = (*witnessNodeMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (w WitnessNode) MarshalJSON() ([]byte, error) {
	type WitnessNode struct {
		Version    uint64         `json:"version"   gencodec:"required"`
		Enode      discover.Node  `json:"node"	  gencodec:"required"`
		Witness    common.Address `json:"witness"   gencodec:"required"`
		UpdateTime time.Time      `json:"updateTime" rlp:"-"`
		Hash       common.Hash    `json:"hash"`
	}
	var enc WitnessNode
	enc.Version = w.Version
	enc.Enode = w.Enode
	enc.Witness = w.Witness
	enc.UpdateTime = w.UpdateTime
	enc.Hash = w.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (w *WitnessNode) UnmarshalJSON(input []byte) error {
	type WitnessNode struct {
		Version    *uint64         `json:"version"   gencodec:"required"`
		Enode      *discover.Node  `json:"node"	  gencodec:"required"`
		Witness    *common.Address `json:"witness"   gencodec:"required"`
		UpdateTime *time.Time      `json:"updateTime" rlp:"-"`
	}
	var dec WitnessNode
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Version == nil {
		return errors.New("missing required field 'version' for WitnessNode")
	}
	w.Version = *dec.Version
	if dec.Enode != nil {
		w.Enode = *dec.Enode
	}
	if dec.Witness == nil {
		return errors.New("missing required field 'witness' for WitnessNode")
	}
	w.Witness = *dec.Witness
	if dec.UpdateTime != nil {
		w.UpdateTime = *dec.UpdateTime
	}
	return nil
}