// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"

	"gitee.com/nerthus/nerthus/common"
)

var _ = (*transactionExecMarshaling)(nil)

// MarshalJSON marshals as JSON.
func (t TransactionExec) MarshalJSON() ([]byte, error) {
	type TransactionExec struct {
		Action  TxAction     `json:"action"          gencodec:"required"`
		PreStep UnitID       `json:"prestep"          gencodec:"required"`
		TxHash  common.Hash  `json:"tx_hash"         gencodec:"required"`
		Tx      *Transaction `json:"tx"`
		Hash    common.Hash  `json:"hash"`
	}
	var enc TransactionExec
	enc.Action = t.Action
	enc.PreStep = t.PreStep
	enc.TxHash = t.TxHash
	enc.Tx = t.Tx
	enc.Hash = t.Hash()
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (t *TransactionExec) UnmarshalJSON(input []byte) error {
	type TransactionExec struct {
		Action  *TxAction    `json:"action"          gencodec:"required"`
		PreStep *UnitID      `json:"prestep"          gencodec:"required"`
		TxHash  *common.Hash `json:"tx_hash"         gencodec:"required"`
		Tx      *Transaction `json:"tx"`
	}
	var dec TransactionExec
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Action == nil {
		return errors.New("missing required field 'action' for TransactionExec")
	}
	t.Action = *dec.Action
	if dec.PreStep == nil {
		return errors.New("missing required field 'prestep' for TransactionExec")
	}
	t.PreStep = *dec.PreStep
	if dec.TxHash == nil {
		return errors.New("missing required field 'tx_hash' for TransactionExec")
	}
	t.TxHash = *dec.TxHash
	if dec.Tx != nil {
		t.Tx = dec.Tx
	}
	return nil
}
