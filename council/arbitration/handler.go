package arbitration

import (
	"crypto/ecdsa"
	"gitee.com/nerthus/nerthus/common"
	core2 "gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"github.com/hashicorp/golang-lru"
)

func NewSchedule(dag *core2.DagChain, signer types.Signer, privKey *ecdsa.PrivateKey, eventMux *event.TypeMux) *ArbitrationSchedule {
	bak := newBackend(dag, signer, privKey, eventMux)
	return &ArbitrationSchedule{
		address: crypto.ForceParsePubKeyToAddress(privKey.PublicKey),
		backend: bak,
	}
}

type ArbitrationSchedule struct {
	address          common.Address
	core             *core
	backend          Backend
	history          *lru.Cache
	activateCallback func()
}

func (t *ArbitrationSchedule) SetActivatedCallback(call func()) {
	t.activateCallback = call
}

func (t *ArbitrationSchedule) Handle(message CouncilMsgEvent) error {
	if t.core == nil {
		t.core = newCore(t.address, t.backend, t.activateCallback)
	}
	if !t.core.Started() {
		t.core.Start()
	}
	return t.core.HandleVote(message.Message)
}

func (t *ArbitrationSchedule) Activating() {
	if t.core == nil || !t.core.Started() {
		t.core = newCore(t.address, t.backend, t.activateCallback)
		t.core.Start()
	}
	t.core.Active()
}
func (t *ArbitrationSchedule) Stop() {
	if t.core != nil && t.core.Started() {
		t.core.Stop()
	}
}
