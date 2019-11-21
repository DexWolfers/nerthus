package bft

import (
	"fmt"
	"gitee.com/nerthus/nerthus/common"
	"math/big"
	"testing"
	"time"
)

func randomAddress(n int) []common.Address {
	var addresses = make([]common.Address, 0, n)
	for i := 1; i <= n; i++ {
		addr := common.BigToAddress(big.NewInt(int64(i)))
		addresses = append(addresses, addr)
	}
	return addresses
}

func TestClacProposer(t *testing.T) {
	addrs := randomAddress(11)
	chains := randomAddress(5)
	v := Validators{}
	for _, c := range addrs {
		//fmt.Println(">>>", c.Hex())
		v = append(v, NewValidator(c))
	}
	last := common.EmptyAddress
	for _, mc := range chains {
		solt := int(fnv32(mc.Hex()))
		fmt.Printf("--------------------chain: %s solt: %d-----------------\n", mc.Hex(), solt)
		for i := 0; i < 6; i++ {
			var newV Validator
			for r := 0; r < 3; r++ {
				newV = nextRoundProposer(v, last, uint64(r), uint64(i+solt))
				fmt.Println(">>>>>>>>", newV.Address().Hex())
			}
			last = newV.Address()
		}
		time.Sleep(time.Second)
	}
}
