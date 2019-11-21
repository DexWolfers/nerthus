// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"github.com/davecgh/go-spew/spew"
)

var unmarshalLogTests = map[string]struct {
	input     string
	want      *Log
	wantError error
}{
	"ok": {
		input: `{"address":"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql","unit_hash":"0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056","unit_number":"0x1ecfa4","data":"0x000000000000000000000000000000000000000000000001a055690d9db80000","log_index":"0x2","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x00000000000000000000000080b2c9d7cbbf30a1b0fc8983c647d754c6525615"],"tx_hash":"0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e","tx_index":"0x3"}`,
		want: &Log{
			Address:   common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql"),
			UnitHash:  common.HexToHash("0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056"),
			UnitIndex: 2019236,
			Data:      hexutil.MustDecode("0x000000000000000000000000000000000000000000000001a055690d9db80000"),
			Index:     2,
			TxIndex:   3,
			TxHash:    common.HexToHash("0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e"),
			Topics: []common.Hash{
				common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
				common.HexToHash("0x00000000000000000000000080b2c9d7cbbf30a1b0fc8983c647d754c6525615"),
			},
		},
	},
	"empty data": {
		input: `{"address":"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql","unit_hash":"0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056","unit_number":"0x1ecfa4","data":"0x","log_index":"0x2","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x00000000000000000000000080b2c9d7cbbf30a1b0fc8983c647d754c6525615"],"tx_hash":"0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e","tx_index":"0x3"}`,
		want: &Log{
			Address:   common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql"),
			UnitHash:  common.HexToHash("0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056"),
			UnitIndex: 2019236,
			Data:      []byte{},
			Index:     2,
			TxIndex:   3,
			TxHash:    common.HexToHash("0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e"),
			Topics: []common.Hash{
				common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
				common.HexToHash("0x00000000000000000000000080b2c9d7cbbf30a1b0fc8983c647d754c6525615"),
			},
		},
	},
	"missing unit fields (pending logs)": {
		input: `{"address":"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql","data":"0x","log_index":"0x0","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"],"tx_hash":"0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e","tx_index":"0x3"}`,
		want: &Log{
			Address:   common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql"),
			UnitHash:  common.Hash{},
			UnitIndex: 0,
			Data:      []byte{},
			Index:     0,
			TxIndex:   3,
			TxHash:    common.HexToHash("0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e"),
			Topics: []common.Hash{
				common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
			},
		},
	},
	"Removed: true": {
		input: `{"address":"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql","unit_hash":"0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056","unit_number":"0x1ecfa4","data":"0x","log_index":"0x2","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"],"tx_hash":"0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e","tx_index":"0x3","removed":true}`,
		want: &Log{
			Address:   common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql"),
			UnitHash:  common.HexToHash("0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056"),
			UnitIndex: 2019236,
			Data:      []byte{},
			Index:     2,
			TxIndex:   3,
			TxHash:    common.HexToHash("0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e"),
			Topics: []common.Hash{
				common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"),
			},
			Removed: true,
		},
	},
	"missing data": {
		input:     `{"address":"nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql","unit_hash":"0x656c34545f90a730a19008c0e7a7cd4fb3895064b48d6d69761bd5abad681056","unit_number":"0x1ecfa4","log_index":"0x2","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x00000000000000000000000080b2c9d7cbbf30a1b0fc8983c647d754c6525615","0x000000000000000000000000f9dff387dcb5cc4cca5b91adb07a95f54e9f1bb6"],"tx_hash":"0x3b198bfd5d2907285af009e9ae84a0ecd63677110d89d7e030251acb87f6487e","tx_index":"0x3"}`,
		wantError: fmt.Errorf("missing required field 'data' for Log"),
	},
}

func TestUnmarshalLog(t *testing.T) {
	dumper := spew.ConfigState{DisableMethods: true, Indent: "    "}
	for name, test := range unmarshalLogTests {
		t.Run(name, func(t *testing.T) {
			var log *Log
			err := json.Unmarshal([]byte(test.input), &log)
			if test.wantError != nil {
				require.EqualError(t, err, test.wantError.Error())
			} else {
				require.NoError(t, err)
			}
			if test.wantError == nil && err == nil {
				if !reflect.DeepEqual(log, test.want) {
					t.Errorf("test %q:\nGOT %sWANT %s", name, dumper.Sdump(log), dumper.Sdump(test.want))
				}
			}
		})

	}
}

func checkError(t *testing.T, testname string, got, want error) bool {
	if got == nil {
		if want != nil {
			t.Errorf("test %q: got no error, want %q", testname, want)
			return false
		}
		return true
	}
	if want == nil {
		t.Errorf("test %q: unexpected error %q", testname, got)
	} else if got.Error() != want.Error() {
		t.Errorf("test %q: got error %q, want %q", testname, got, want)
	}
	return false
}
