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
	"fmt"
	"io"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/rlp"
)

//go:generate gencodec -type Log -field-override logMarshaling -out gen_log_json.go

// Log represents a contract log event. These events are generated by the LOG opcode and
// stored/indexed by the node.
type Log struct {
	// Consensus fields:
	// address of the contract that generated the event
	Address common.Address `json:"address" gencodec:"required"`
	// list of topics provided by the contract.
	Topics []common.Hash `json:"topics" gencodec:"required"`
	// supplied by the contract, usually ABI-encoded
	Data []byte `json:"data" gencodec:"required"`

	// Derived fields. These fields are filled in by the node
	// but not secured by consensus.
	// unit in which the transaction was included
	//BlockNumber uint64 `json:"blockNumber"`
	// hash of the transaction
	TxHash common.Hash `json:"tx_hash" gencodec:"required"`
	// index of the transaction in the unit
	TxIndex uint `json:"tx_index" gencodec:"required"`
	// hash of the unit in which the transaction was included
	UnitHash  common.Hash `json:"unit_hash"`
	UnitIndex uint64      `json:"unit_number"`
	// index of the log in the receipt
	Index uint `json:"log_index" gencodec:"required"`

	// The Removed field is true if this log was reverted due to a chain reorganisation.
	// You must pay attention to this field if you receive logs through a filter query.
	Removed bool `json:"removed"`
}

type logMarshaling struct {
	Data      hexutil.Bytes
	TxIndex   hexutil.Uint
	Index     hexutil.Uint
	UnitIndex hexutil.Uint
}

type rlpLog struct {
	Address common.Address
	Topics  []common.Hash
	Data    []byte
}

type rlpStorageLog struct {
	Address   common.Address
	Topics    []common.Hash
	Data      []byte
	UnitIndex uint
	TxHash    common.Hash
	TxIndex   uint
	BlockHash common.Hash
	Index     uint
}

// EncodeRLP implements rlp.Encoder.
func (l *Log) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpLog{Address: l.Address, Topics: l.Topics, Data: l.Data})
}

// DecodeRLP implements rlp.Decoder.
func (l *Log) DecodeRLP(s *rlp.Stream) error {
	var dec rlpLog
	err := s.Decode(&dec)
	if err == nil {
		l.Address, l.Topics, l.Data = dec.Address, dec.Topics, dec.Data
	}
	return err
}

func (l *Log) String() string {
	return fmt.Sprintf(`log: %x %x %x %x %d %x %d`, l.Address, l.Topics, l.Data, l.TxHash, l.TxIndex, l.UnitHash, l.Index)
}

func (l *Log) ContainsTopic(t common.Hash) bool {
	for _, v := range l.Topics {
		if v == t {
			return true
		}
	}
	return false
}

// LogForStorage is a wrapper around a Log that flattens and parses the entire content of
// a log including non-consensus fields.
type LogForStorage Log

// EncodeRLP implements rlp.Encoder.
func (l *LogForStorage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, rlpStorageLog{
		Address:   l.Address,
		Topics:    l.Topics,
		Data:      l.Data,
		TxHash:    l.TxHash,
		TxIndex:   l.TxIndex,
		BlockHash: l.UnitHash,
		Index:     l.Index,
	})
}

// DecodeRLP implements rlp.Decoder.
func (l *LogForStorage) DecodeRLP(s *rlp.Stream) error {
	var dec rlpStorageLog
	err := s.Decode(&dec)
	if err == nil {
		*l = LogForStorage{
			Address:  dec.Address,
			Topics:   dec.Topics,
			Data:     dec.Data,
			TxHash:   dec.TxHash,
			TxIndex:  dec.TxIndex,
			UnitHash: dec.BlockHash,
			Index:    dec.Index,
		}
	}
	return err
}
