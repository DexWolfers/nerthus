package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGasRemaining(t *testing.T) {
	caces := []struct {
		userGas uint64
		freeGas uint64
		usedGas uint64
		refund  int64
	}{
		{100, 40, 1, 100},
		{100, 40, 1, 100},
		{100, 40, 40, 100},
		{100, 40, 80, 60},
		{100, 40, 100, 40},
		{100, 40, 101, 39},
		{100, 40, 120, 20},

		{1, 40, 41, 0},
		{1, 40, 40, 1},
		{1, 40, 20, 1},
	}

	for i, c := range caces {
		s := &StateTransition{
			initialGas: c.freeGas + c.userGas,
			freeGas:    c.freeGas,
			gasPrice:   1,
		}
		s.gas = s.initialGas

		s.useGas(c.usedGas)
		require.Equal(t, c.refund, s.calcRefundValue().Int64(), "at cace %d", i)

	}
}
