// Author: @ysqi

package core

import "errors"

var (
	// ErrBlacklistedHash is returned if a unit to import is on the blacklist.
	ErrBlacklistedHash = errors.New("blacklisted hash")

	ErrChainConfigNotFound = errors.New("ChainConfig not found") // general config not found error

	// ErrGasLimitReached is returned by the gas pool if the amount of gas required
	// by a transaction is higher than what's left in the unit.
	ErrGasLimitReached = errors.New("gas limit reached")
)
