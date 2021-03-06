// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
)

var _ = (*logMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (l Log) MarshalJSON() ([]byte, error) {
	type Log struct {
		Address   common.Address `json:"address" gencodec:"required"`
		Topics    []common.Hash  `json:"topics" gencodec:"required"`
		Data      hexutil.Bytes  `json:"data" gencodec:"required"`
		TxHash    common.Hash    `json:"tx_hash" gencodec:"required"`
		TxIndex   hexutil.Uint   `json:"tx_index" gencodec:"required"`
		UnitHash  common.Hash    `json:"unit_hash"`
		UnitIndex hexutil.Uint   `json:"unit_number"`
		Index     hexutil.Uint   `json:"log_index" gencodec:"required"`
		Removed   bool           `json:"removed"`
	}
	var enc Log
	enc.Address = l.Address
	enc.Topics = l.Topics
	enc.Data = l.Data
	enc.TxHash = l.TxHash
	enc.TxIndex = hexutil.Uint(l.TxIndex)
	enc.UnitHash = l.UnitHash
	enc.UnitIndex = hexutil.Uint(l.UnitIndex)
	enc.Index = hexutil.Uint(l.Index)
	enc.Removed = l.Removed
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (l *Log) UnmarshalJSON(input []byte) error {
	type Log struct {
		Address   *common.Address `json:"address" gencodec:"required"`
		Topics    []common.Hash   `json:"topics" gencodec:"required"`
		Data      *hexutil.Bytes  `json:"data" gencodec:"required"`
		TxHash    *common.Hash    `json:"tx_hash" gencodec:"required"`
		TxIndex   *hexutil.Uint   `json:"tx_index" gencodec:"required"`
		UnitHash  *common.Hash    `json:"unit_hash"`
		UnitIndex *hexutil.Uint   `json:"unit_number"`
		Index     *hexutil.Uint   `json:"log_index" gencodec:"required"`
		Removed   *bool           `json:"removed"`
	}
	var dec Log
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Address == nil {
		return errors.New("missing required field 'address' for Log")
	}
	l.Address = *dec.Address
	if dec.Topics == nil {
		return errors.New("missing required field 'topics' for Log")
	}
	l.Topics = dec.Topics
	if dec.Data == nil {
		return errors.New("missing required field 'data' for Log")
	}
	l.Data = *dec.Data
	if dec.TxHash == nil {
		return errors.New("missing required field 'tx_hash' for Log")
	}
	l.TxHash = *dec.TxHash
	if dec.TxIndex == nil {
		return errors.New("missing required field 'tx_index' for Log")
	}
	l.TxIndex = uint(*dec.TxIndex)
	if dec.UnitHash != nil {
		l.UnitHash = *dec.UnitHash
	}
	if dec.UnitIndex != nil {
		l.UnitIndex = uint64(*dec.UnitIndex)
	}
	if dec.Index == nil {
		return errors.New("missing required field 'log_index' for Log")
	}
	l.Index = uint(*dec.Index)
	if dec.Removed != nil {
		l.Removed = *dec.Removed
	}
	return nil
}
