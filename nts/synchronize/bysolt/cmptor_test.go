package bysolt

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/log"
	"github.com/stretchr/testify/require"
)

func TestLastStatus(t *testing.T) {
	cluster := startCluster()
	p1 := newPeer(1, cluster)
	p1.points = make([][]common.Hash, 100)
	for i := 0; i < len(p1.points); i++ {
		hashes := make([]common.Hash, 10)
		for j := 1; j <= len(hashes); j++ {
			hashes[j-1] = common.BigToHash(big.NewInt(int64(j)))
		}
		p1.points[i] = hashes
	}
	last := p1.LastPointStatus()
	t.Log(last)
	preLast := p1.GetPointStatus(last.Number - 1)
	require.Equal(t, last.Parent, preLast.Root, "diff parent hash with last")
}

func TestCmp(t *testing.T) {
	testutil.SetupTestLogging()
	cluster := startCluster()
	p1 := newPeer(1, cluster)
	p1.points = make([][]common.Hash, 1)

	p2 := newPeer(2, cluster)
	p2.points = make([][]common.Hash, 1)
	for i := 1; i < 100; i++ {
		if i%5 == 0 {
			p1.points[0] = append(p1.points[0], common.BigToHash(big.NewInt(int64(i))))
		} else if i%7 == 0 {
			p2.points[0] = append(p2.points[0], common.BigToHash(big.NewInt(int64(i))))
		} else {
			p1.points[0] = append(p1.points[0], common.BigToHash(big.NewInt(int64(i))))
			p2.points[0] = append(p2.points[0], common.BigToHash(big.NewInt(int64(i))))
		}
	}

	createCmptor := func(p *TestBackend) *treeCmptor {
		tree1, _ := p.GetPointTree(uint64(len(p.points)))
		return newTreeCmptor(tree1, func(number uint64) NodeStatus {
			if number == 0 {
				return p.LastPointStatus()
			} else {
				return p.GetPointStatus(number)
			}
		}, p, p)
	}

	pm1 := createCmptor(p1)
	p1.handle = func(event *RealtimeEvent) error {
		log.Trace("---------------p1 handle msg--------------")
		return pm1.handleResponse(event)
	}
	pm2 := createCmptor(p2)
	p2.handle = func(event *RealtimeEvent) error {
		log.Trace("---------------p2 handle msg--------------")
		return pm2.handleResponse(event)
	}

	log.Trace("ready to go ", "p1", p1.id, "status1", p1.LastPointStatus(), "p2", p2.id, "status2", p2.LastPointStatus())

	quit1 := make(chan struct{})
	pm1.start(&RealtimeEvent{
		Code:         RealtimeRequest,
		From:         p2.id,
		RemoteStatus: p2.LastPointStatus(),
	}, quit1)

	quit2 := make(chan struct{})
	pm2.start(&RealtimeEvent{
		Code:         RealtimeReponse,
		From:         p1.id,
		RemoteStatus: p1.LastPointStatus(),
	}, quit2)

	<-quit1
	log.Debug("p1 quit")
	<-quit2
	log.Debug("p2 quit")
	require.Equal(t, p1.LastPointStatus(), p2.LastPointStatus())
}
