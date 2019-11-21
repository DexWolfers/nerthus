package sc

var CounterItem = Counter{}

type Counter struct{}

var counterItem = SubItems([]interface{}{"all_counter"})

func (c Counter) ActiveWtiness(db StateDB) uint64 {
	return counterItem.GetSub(db, "witness_actives").Big().Uint64()
}
func (c Counter) AddActiveWitness(db StateDB, n int) {
	counterItem.SaveSub(db, "witness_actives", safeAdd(c.ActiveWtiness(db), n))
}
