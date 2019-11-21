package sc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSCAPI_GetAbiCode(t *testing.T) {
	api := SCAPI{}
	s, err := api.GetAbiCode()
	require.Nil(t, err)
	t.Log(s)
}
