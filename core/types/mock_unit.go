package types

import "gitee.com/nerthus/nerthus/common"

type MockUnit struct {
	*Unit
}

func NewMockUnit(mc common.Address) *MockUnit {
	mockUnit := MockUnit{
		Unit: &Unit{
			header: &Header{
				MC: mc,
			},
		},
	}
	return &mockUnit
}
