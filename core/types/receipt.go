// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/rlp"
)

//go:generate gencodec -type Receipt -field-override receiptMarshaling -out gen_receipt_json.go

var (
	receiptStatusFailed     = common.Hash{}
	receiptStatusSuccessful = common.BytesToHash([]byte{0x01})
)

// Receipt represents the results of a transaction.
type Receipt struct {
	// Consensus fields
	Failed            bool           `json:"failed" gencodec:"required"`
	CumulativeGasUsed *big.Int       `json:"cumulative_gasUsed" gencodec:"required"`
	Bloom             Bloom          `json:"logs_bloom"         gencodec:"required"`
	Logs              []*Log         `json:"logs"              gencodec:"required"`
	Action            TxAction       `json:"action"    gencodec:"required"`
	PreStep           UnitID         `json:"pre_step"  gencodec:"required"`
	TxHash            common.Hash    `json:"tx_hash" gencodec:"required"`
	ContractAddress   common.Address `json:"contract_address"`
	GasRemaining      uint64         `json:"gas_remaining"  gencodec:"required"` //剩余Gas
	GasUsed           uint64         `json:"gas_used" gencodec:"required"`
	Coinflows         Coinflows      `json:"coinflows"` // 账务输出
	Output            []byte         `json:"output"`
}

type receiptMarshaling struct {
	CumulativeGasUsed *hexutil.Big
	GasUsed           hexutil.Uint64
	GasRemaining      hexutil.Uint64
	Output            hexutil.Bytes
}

type receiptStorageRLP struct {
	Failed            bool
	Action            TxAction
	CumulativeGasUsed *big.Int
	GasUsed           uint64
	GasRemaining      uint64
	PreStep           UnitID
	ContractAddress   common.Address
	TxHash            common.Hash
	Coinflows         Coinflows
	Bloom             Bloom
	Logs              []*LogForStorage
	Output            []byte
}

// NewReceipt creates a barebone transaction receipt, copying the init fields.
func NewReceipt(action TxAction, root common.Hash, failed bool, cumulativeGasUsed *big.Int) *Receipt {
	return &Receipt{Action: action, Failed: failed, CumulativeGasUsed: new(big.Int).Set(cumulativeGasUsed)}
}

// String implements the Stringer interface.
func (r *Receipt) String() string {
	return fmt.Sprintf("receipt{failed=%t action=%s cgas=%v bloom=%x logs=%v}", r.Failed, r.Action, r.CumulativeGasUsed, r.Bloom, r.Logs)
}

// ReceiptForStorage is a wrapper around a Receipt that flattens and parses the
// entire content of a receipt, as opposed to only the consensus fields originally.
type ReceiptForStorage Receipt

// EncodeRLP implements rlp.Encoder, and flattens all content fields of a receipt
// into an RLP stream.
func (r *ReceiptForStorage) EncodeRLP(w io.Writer) error {
	enc := &receiptStorageRLP{
		Failed:            r.Failed,
		Action:            r.Action,
		CumulativeGasUsed: r.CumulativeGasUsed,
		Bloom:             r.Bloom,
		TxHash:            r.TxHash,
		ContractAddress:   r.ContractAddress,
		Logs:              make([]*LogForStorage, len(r.Logs)),
		GasUsed:           r.GasUsed,
		GasRemaining:      r.GasRemaining,
		Coinflows:         r.Coinflows,
		Output:            r.Output,
		PreStep:           r.PreStep,
	}
	for i, log := range r.Logs {
		enc.Logs[i] = (*LogForStorage)(log)
	}
	return rlp.Encode(w, enc)
}

// DecodeRLP implements rlp.Decoder, and loads both consensus and implementation
// fields of a receipt from an RLP stream.
func (r *ReceiptForStorage) DecodeRLP(s *rlp.Stream) error {
	var dec receiptStorageRLP
	if err := s.Decode(&dec); err != nil {
		return err
	}
	r.Failed = dec.Failed
	// Assign the consensus fields
	r.CumulativeGasUsed, r.Bloom, r.Action = dec.CumulativeGasUsed, dec.Bloom, dec.Action
	r.Logs = make([]*Log, len(dec.Logs))
	for i, log := range dec.Logs {
		r.Logs[i] = (*Log)(log)
	}
	// Assign the implementation fields
	r.TxHash, r.ContractAddress, r.GasUsed, r.GasRemaining = dec.TxHash, dec.ContractAddress, dec.GasUsed, dec.GasRemaining
	r.Coinflows = dec.Coinflows
	r.Output = dec.Output
	r.PreStep = dec.PreStep
	return nil
}

// Receipts is a wrapper around a Receipt array to implement DerivableList.
type Receipts []*Receipt

// Len returns the number of receipts in this list.
func (r Receipts) Len() int { return len(r) }

// GetRlp returns the RLP encoding of one receipt from the list.
func (r Receipts) GetRlp(i int) []byte {
	bytes, err := rlp.EncodeToBytes(r[i])
	if err != nil {
		info, _ := json.MarshalIndent(r[i], "", "\t")
		panic(fmt.Sprintf("rlp error:%v\n%s", err, info))
	}
	return bytes
}
