// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
)

var _ = (*headerMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (h Header) MarshalJSON() ([]byte, error) {
	type Header struct {
		MC          common.Address `json:"mc" gencodec:"required"`
		Number      hexutil.Uint64 `json:"number" gencodec:"required"`
		ParentHash  common.Hash    `json:"parent_hash" gencodec:"required"`
		SCHash      common.Hash    `json:"sc_hash" gencodec:"required"`
		SCNumber    hexutil.Uint64 `json:"sc_number" gencodec:"required"`
		Proposer    common.Address `json:"proposer" gencodec:"required"`
		TxsRoot     common.Hash    `json:"transactions_root" gencodec:"required"`
		ReceiptRoot common.Hash    `json:"receipt_root" gencodec:"required"`
		StateRoot   common.Hash    `json:"state_root" gencodec:"required"`
		Bloom       Bloom          `json:"bloom"`
		GasLimit    hexutil.Uint64 `json:"gas_limit" gencodec:"required"`
		GasUsed     hexutil.Uint64 `json:"gas_used" gencodec:"required"`
		Timestamp   hexutil.Uint64 `json:"timestamp" gencodec:"required"`
		Hash        common.Hash    `json:"hash"`
	}
	var enc Header
	enc.MC = h.MC
	enc.Number = hexutil.Uint64(h.Number)
	enc.ParentHash = h.ParentHash
	enc.SCHash = h.SCHash
	enc.SCNumber = hexutil.Uint64(h.SCNumber)
	enc.Proposer = h.Proposer
	enc.TxsRoot = h.TxsRoot
	enc.ReceiptRoot = h.ReceiptRoot
	enc.StateRoot = h.StateRoot
	enc.Bloom = h.Bloom
	enc.GasLimit = hexutil.Uint64(h.GasLimit)
	enc.GasUsed = hexutil.Uint64(h.GasUsed)
	enc.Timestamp = hexutil.Uint64(h.Timestamp)
	enc.Hash = h.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (h *Header) UnmarshalJSON(input []byte) error {
	type Header struct {
		MC          *common.Address `json:"mc" gencodec:"required"`
		Number      *hexutil.Uint64 `json:"number" gencodec:"required"`
		ParentHash  *common.Hash    `json:"parent_hash" gencodec:"required"`
		SCHash      *common.Hash    `json:"sc_hash" gencodec:"required"`
		SCNumber    *hexutil.Uint64 `json:"sc_number" gencodec:"required"`
		Proposer    *common.Address `json:"proposer" gencodec:"required"`
		TxsRoot     *common.Hash    `json:"transactions_root" gencodec:"required"`
		ReceiptRoot *common.Hash    `json:"receipt_root" gencodec:"required"`
		StateRoot   *common.Hash    `json:"state_root" gencodec:"required"`
		Bloom       *Bloom          `json:"bloom"`
		GasLimit    *hexutil.Uint64 `json:"gas_limit" gencodec:"required"`
		GasUsed     *hexutil.Uint64 `json:"gas_used" gencodec:"required"`
		Timestamp   *hexutil.Uint64 `json:"timestamp" gencodec:"required"`
	}
	var dec Header
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.MC == nil {
		return errors.New("missing required field 'mc' for Header")
	}
	h.MC = *dec.MC
	if dec.Number == nil {
		return errors.New("missing required field 'number' for Header")
	}
	h.Number = uint64(*dec.Number)
	if dec.ParentHash == nil {
		return errors.New("missing required field 'parent_hash' for Header")
	}
	h.ParentHash = *dec.ParentHash
	if dec.SCHash == nil {
		return errors.New("missing required field 'sc_hash' for Header")
	}
	h.SCHash = *dec.SCHash
	if dec.SCNumber == nil {
		return errors.New("missing required field 'sc_number' for Header")
	}
	h.SCNumber = uint64(*dec.SCNumber)
	if dec.Proposer == nil {
		return errors.New("missing required field 'proposer' for Header")
	}
	h.Proposer = *dec.Proposer
	if dec.TxsRoot == nil {
		return errors.New("missing required field 'transactions_root' for Header")
	}
	h.TxsRoot = *dec.TxsRoot
	if dec.ReceiptRoot == nil {
		return errors.New("missing required field 'receipt_root' for Header")
	}
	h.ReceiptRoot = *dec.ReceiptRoot
	if dec.StateRoot == nil {
		return errors.New("missing required field 'state_root' for Header")
	}
	h.StateRoot = *dec.StateRoot
	if dec.Bloom != nil {
		h.Bloom = *dec.Bloom
	}
	if dec.GasLimit == nil {
		return errors.New("missing required field 'gas_limit' for Header")
	}
	h.GasLimit = uint64(*dec.GasLimit)
	if dec.GasUsed == nil {
		return errors.New("missing required field 'gas_used' for Header")
	}
	h.GasUsed = uint64(*dec.GasUsed)
	if dec.Timestamp == nil {
		return errors.New("missing required field 'timestamp' for Header")
	}
	h.Timestamp = uint64(*dec.Timestamp)
	return nil
}
