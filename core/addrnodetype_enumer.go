// Code generated by "enumer -type=AddrNodeType -json -transform=snake -trimprefix=AddrNodeType"; DO NOT EDIT.

package core

import (
	"encoding/json"
	"fmt"
)

const _AddrNodeTypeName = "offlinejoin_underwaystopstart"

var _AddrNodeTypeIndex = [...]uint8{0, 7, 20, 24, 29}

func (i AddrNodeType) String() string {
	if i >= AddrNodeType(len(_AddrNodeTypeIndex)-1) {
		return fmt.Sprintf("AddrNodeType(%d)", i)
	}
	return _AddrNodeTypeName[_AddrNodeTypeIndex[i]:_AddrNodeTypeIndex[i+1]]
}

var _AddrNodeTypeValues = []AddrNodeType{0, 1, 2, 3}

var _AddrNodeTypeNameToValueMap = map[string]AddrNodeType{
	_AddrNodeTypeName[0:7]:   0,
	_AddrNodeTypeName[7:20]:  1,
	_AddrNodeTypeName[20:24]: 2,
	_AddrNodeTypeName[24:29]: 3,
}

// AddrNodeTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func AddrNodeTypeString(s string) (AddrNodeType, error) {
	if val, ok := _AddrNodeTypeNameToValueMap[s]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to AddrNodeType values", s)
}

// AddrNodeTypeValues returns all values of the enum
func AddrNodeTypeValues() []AddrNodeType {
	return _AddrNodeTypeValues
}

// IsAAddrNodeType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i AddrNodeType) IsAAddrNodeType() bool {
	for _, v := range _AddrNodeTypeValues {
		if i == v {
			return true
		}
	}
	return false
}

// MarshalJSON implements the json.Marshaler interface for AddrNodeType
func (i AddrNodeType) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface for AddrNodeType
func (i *AddrNodeType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("AddrNodeType should be a string, got %s", data)
	}

	var err error
	*i, err = AddrNodeTypeString(s)
	return err
}
