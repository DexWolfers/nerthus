// Code generated by "enumer -type TxAction -json -transform=snake -trimprefix=Action"; DO NOT EDIT.

package types

import (
	"encoding/json"
	"fmt"
)

const _TxActionName = "transfer_paymenttransfer_receiptcontract_freezecontract_dealpow_tx_contract_dealsc_idle_contract_dealcontract_refundcontract_receipt"

var _TxActionIndex = [...]uint8{0, 16, 32, 47, 60, 80, 101, 116, 132}

func (i TxAction) String() string {
	if i >= TxAction(len(_TxActionIndex)-1) {
		return fmt.Sprintf("TxAction(%d)", i)
	}
	return _TxActionName[_TxActionIndex[i]:_TxActionIndex[i+1]]
}

var _TxActionValues = []TxAction{0, 1, 2, 3, 4, 5, 6, 7}

var _TxActionNameToValueMap = map[string]TxAction{
	_TxActionName[0:16]:    0,
	_TxActionName[16:32]:   1,
	_TxActionName[32:47]:   2,
	_TxActionName[47:60]:   3,
	_TxActionName[60:80]:   4,
	_TxActionName[80:101]:  5,
	_TxActionName[101:116]: 6,
	_TxActionName[116:132]: 7,
}

// TxActionString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func TxActionString(s string) (TxAction, error) {
	if val, ok := _TxActionNameToValueMap[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to TxAction values", s)
}

// TxActionValues returns all values of the enum
func TxActionValues() []TxAction {
	return _TxActionValues
}

// IsATxAction returns "true" if the value is listed in the enum definition. "false" otherwise
func (i TxAction) IsATxAction() bool {
	for _, v := range _TxActionValues {
		if i == v {
			return true
		}
	}
	return false
}

// MarshalJSON implements the json.Marshaler interface for TxAction
func (i TxAction) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface for TxAction
func (i *TxAction) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("TxAction should be a string, got %s", data)
	}

	var err error
	*i, err = TxActionString(s)
	return err
}
