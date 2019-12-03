package params

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDOT2NTS(t *testing.T) {
	nts1 := new(big.Int).SetUint64(Nts)
	dot := NTS2DOT(nts1)
	nts := DOT2NTS(dot)
	require.Equal(t, nts1, nts)
}
