package params

import (
	"math/big"
)

var (
	MaxUint256 = new(big.Int).Exp(big.NewInt(2), big.NewInt(256), big.NewInt(0))
)
