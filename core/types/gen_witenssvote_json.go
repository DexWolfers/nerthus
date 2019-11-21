// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package types

import (
	"encoding/json"
	"errors"
)

// MarshalJSON marshals as JSON.
func (w WitenssVote) MarshalJSON() ([]byte, error) {
	type WitenssVote struct {
		Extra []byte      `json:"extra" gencodec:"required"`
		Sign  SignContent `json:"sign"             gencodec:"required"`
	}
	var enc WitenssVote
	enc.Extra = w.Extra
	enc.Sign = w.Sign
	return json.Marshal(&enc)
}

// UnmarshalJSON unmarshals from JSON.
func (w *WitenssVote) UnmarshalJSON(input []byte) error {
	type WitenssVote struct {
		Extra []byte       `json:"extra" gencodec:"required"`
		Sign  *SignContent `json:"sign"             gencodec:"required"`
	}
	var dec WitenssVote
	if err := json.Unmarshal(input, &dec); err != nil {
		return err
	}
	if dec.Extra == nil {
		return errors.New("missing required field 'extra' for WitenssVote")
	}
	w.Extra = dec.Extra
	if dec.Sign == nil {
		return errors.New("missing required field 'sign' for WitenssVote")
	}
	w.Sign = *dec.Sign
	return nil
}