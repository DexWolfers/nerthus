package monitor

//go:generate enumer -type=BftEventCode -json -transform=snake

type ChainStatus uint8

const (
	StatusRunning ChainStatus = iota
	StatusError
	StatusStop
)
