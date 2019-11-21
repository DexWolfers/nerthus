package bft

import (
	"errors"
	"fmt"
)

var (
	// ErrUnauthorizedAddress is returned when given address cannot be found in
	// current validator set.
	ErrUnauthorizedAddress = errors.New("unauthorized address")
	// ErrNilValidatorSet
	ErrNilValidatorSet = errors.New("validator set is nil")
	// ErrStoppedEngine is returned if the engine is stopped
	ErrStoppedEngine = errors.New("stopped engine")
	// ErrStartedEngine is returned if the engine is already started
	ErrStartedEngine = errors.New("started engine")
	// ErrExistRoundChange is have received roundchange from given addr
	ErrExistVoteMsg = errors.New("already have msg from that address")
)

func PanicSanity(v interface{}) {
	panic(fmt.Sprintf("Panicked on a Sanity Check: %v", v))
}
