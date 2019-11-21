package types

import (
	"encoding/json"
	"errors"

	"gitee.com/nerthus/nerthus/common"
)

func (b Unit) MarshalJSON() ([]byte, error) {
	type Unit struct {
		Header         *Header          `json:"header"`
		Transactions   TxExecs          `json:"transactions"`
		WitnessesSigns []WitenssVote    `json:"witness_sign"`
		Witnesses      []common.Address `json:"witnesses"`
		Sign           SignContent      `json:"sign"`
	}
	var enc Unit
	enc.Sign = b.Sign
	enc.Header = b.header
	enc.Transactions = b.transactions
	// JSON 时不在需要重复输出单元头哈希
	enc.WitnessesSigns = b.witenssVotes
	enc.Witnesses, _ = b.Witnesses()
	return json.Marshal(&enc)
}

func (b *Unit) UnmarshalJSON(input []byte) error {
	type Unit struct {
		Header       *Header       `json:"header"`
		Transactions TxExecs       `json:"transactions"`
		Witnesses    []WitenssVote `json:"witness_sign"`
		Sign         *SignContent  `json:"sign"`
	}
	var dec Unit
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}

	if dec.Sign == nil {
		return errors.New("missing required field 'sign' for Unit")
	} else {
		b.Sign = *dec.Sign
		//检查签名
		if err := deriveSigner(b.Sign.V()).ValidateSignatureValues(b); err != nil {
			return err
		}
	}
	b.header = dec.Header
	b.witenssVotes = dec.Witnesses
	b.transactions = dec.Transactions
	return nil
}
