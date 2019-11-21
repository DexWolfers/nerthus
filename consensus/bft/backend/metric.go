package backend

import (
	"fmt"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"strings"

	"gitee.com/nerthus/nerthus/common/chancluster"
)

type activeChainsVar struct {
	objs *objectpool.ObjectPool
	clt  *chancluster.ChanCluster
}

func (a *activeChainsVar) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, ",%q:%d", "objs", a.objs.Count())
	fmt.Fprintf(&b, ",%q:%d", "wait", a.objs.WaitCount())
	fmt.Fprintf(&b, ",%q:%d}", "gos", a.clt.ParallelCount())
	return b.String()
}
