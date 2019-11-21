// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
)

var _ = (*receiptMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (r Receipt) MarshalJSON() ([]byte, error) {
	type Receipt struct {
		Failed            bool           `json:"failed" gencodec:"required"`
		CumulativeGasUsed *hexutil.Big   `json:"cumulative_gasUsed" gencodec:"required"`
		Bloom             Bloom          `json:"logs_bloom"         gencodec:"required"`
		Logs              []*Log         `json:"logs"              gencodec:"required"`
		Action            TxAction       `json:"action"    gencodec:"required"`
		PreStep           UnitID         `json:"pre_step"  gencodec:"required"`
		TxHash            common.Hash    `json:"tx_hash" gencodec:"required"`
		ContractAddress   common.Address `json:"contract_address"`
		GasRemaining      hexutil.Uint64 `json:"gas_remaining"  gencodec:"required"`
		GasUsed           hexutil.Uint64 `json:"gas_used" gencodec:"required"`
		Coinflows         Coinflows      `json:"coinflows"`
		Output            hexutil.Bytes  `json:"output"`
	}
	var enc Receipt
	enc.Failed = r.Failed
	enc.CumulativeGasUsed = (*hexutil.Big)(r.CumulativeGasUsed)
	enc.Bloom = r.Bloom
	enc.Logs = r.Logs
	enc.Action = r.Action
	enc.PreStep = r.PreStep
	enc.TxHash = r.TxHash
	enc.ContractAddress = r.ContractAddress
	enc.GasRemaining = hexutil.Uint64(r.GasRemaining)
	enc.GasUsed = hexutil.Uint64(r.GasUsed)
	enc.Coinflows = r.Coinflows
	enc.Output = r.Output
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (r *Receipt) UnmarshalJSON(input []byte) error {
	type Receipt struct {
		Failed            *bool           `json:"failed" gencodec:"required"`
		CumulativeGasUsed *hexutil.Big    `json:"cumulative_gasUsed" gencodec:"required"`
		Bloom             *Bloom          `json:"logs_bloom"         gencodec:"required"`
		Logs              []*Log          `json:"logs"              gencodec:"required"`
		Action            *TxAction       `json:"action"    gencodec:"required"`
		PreStep           *UnitID         `json:"pre_step"  gencodec:"required"`
		TxHash            *common.Hash    `json:"tx_hash" gencodec:"required"`
		ContractAddress   *common.Address `json:"contract_address"`
		GasRemaining      *hexutil.Uint64 `json:"gas_remaining"  gencodec:"required"`
		GasUsed           *hexutil.Uint64 `json:"gas_used" gencodec:"required"`
		Coinflows         *Coinflows      `json:"coinflows"`
		Output            *hexutil.Bytes  `json:"output"`
	}
	var dec Receipt
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Failed == nil {
		return errors.New("missing required field 'failed' for Receipt")
	}
	r.Failed = *dec.Failed
	if dec.CumulativeGasUsed == nil {
		return errors.New("missing required field 'cumulative_gasUsed' for Receipt")
	}
	r.CumulativeGasUsed = (*big.Int)(dec.CumulativeGasUsed)
	if dec.Bloom == nil {
		return errors.New("missing required field 'logs_bloom' for Receipt")
	}
	r.Bloom = *dec.Bloom
	if dec.Logs == nil {
		return errors.New("missing required field 'logs' for Receipt")
	}
	r.Logs = dec.Logs
	if dec.Action == nil {
		return errors.New("missing required field 'action' for Receipt")
	}
	r.Action = *dec.Action
	if dec.PreStep == nil {
		return errors.New("missing required field 'pre_step' for Receipt")
	}
	r.PreStep = *dec.PreStep
	if dec.TxHash == nil {
		return errors.New("missing required field 'tx_hash' for Receipt")
	}
	r.TxHash = *dec.TxHash
	if dec.ContractAddress != nil {
		r.ContractAddress = *dec.ContractAddress
	}
	if dec.GasRemaining == nil {
		return errors.New("missing required field 'gas_remaining' for Receipt")
	}
	r.GasRemaining = uint64(*dec.GasRemaining)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gas_used' for Receipt")
	}
	r.GasUsed = uint64(*dec.GasUsed)
	if dec.Coinflows != nil {
		r.Coinflows = *dec.Coinflows
	}
	if dec.Output != nil {
		r.Output = *dec.Output
	}
	return nil
}
