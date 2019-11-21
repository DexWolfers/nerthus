package state

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"

	"gopkg.in/check.v1"
)

// Tests that updating a state trie does not leak any database writes prior to
// actually committing the state.
func TestUpdateLeaks(t *testing.T) {
	// Create an empty state database
	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	// Update it with some accounts
	for i := byte(0); i < 255; i++ {
		addr := common.BytesToAddress([]byte{i})
		state.AddBalance(addr, big.NewInt(int64(11*i)))
		state.SetNonce(addr, uint64(42*i))
		if i%2 == 0 {
			state.SetState(addr, common.BytesToHash([]byte{i, i, i}), common.BytesToHash([]byte{i, i, i, i}))
		}
		if i%3 == 0 {
			state.SetCode(addr, []byte{i, i, i, i, i})
		}
		state.IntermediateRoot(common.Address{})
	}
	// Ensure that no data was leaked into the database
	for _, key := range db.Keys() {
		value, _ := db.Get(key)
		t.Errorf("State leaked into database: %x -> %x", key, value)
	}
}

// Tests that no intermediate state of an object is stored into the database,
// only the one right before the commit.
func TestIntermediateLeaks(t *testing.T) {
	// Create two state databases, one transitioning to the final state, the other final from the beginning
	transDb, _ := ntsdb.NewMemDatabase()
	finalDb, _ := ntsdb.NewMemDatabase()
	transState, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(transDb))
	finalState, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(finalDb))

	modify := func(state *StateDB, addr common.Address, i, tweak byte) {
		state.SetBalance(addr, big.NewInt(int64(11*i)+int64(tweak)))
		state.SetNonce(addr, uint64(42*i+tweak))
		if i%2 == 0 {
			state.SetState(addr, common.Hash{i, i, i, 0}, common.Hash{})
			state.SetState(addr, common.Hash{i, i, i, tweak}, common.Hash{i, i, i, i, tweak})
		}
		if i%3 == 0 {
			state.SetCode(addr, []byte{i, i, i, i, i, tweak})
		}
	}

	// Modify the transient state.
	for i := byte(0); i < 255; i++ {
		modify(transState, newAddr(i), i, 0)
	}
	// Write modifications to trie.
	transState.IntermediateRoot(common.Address{})

	// Overwrite all the data with new values in the transient database.
	for i := byte(0); i < 255; i++ {
		modify(transState, newAddr(i), i, 99)
		modify(finalState, newAddr(i), i, 99)
	}

	// Commit and cross check the databases.
	if _, err := transState.CommitTo(transDb); err != nil {
		t.Fatalf("failed to commit transition state: %v", err)
	}
	if _, err := finalState.CommitTo(finalDb); err != nil {
		t.Fatalf("failed to commit final state: %v", err)
	}
	for _, key := range finalDb.Keys() {
		if _, err := transDb.Get(key); err != nil {
			val, _ := finalDb.Get(key)
			t.Errorf("entry missing from the transition database: %x -> %x", key, val)
		}
	}
	for _, key := range transDb.Keys() {
		if _, err := finalDb.Get(key); err != nil {
			val, _ := transDb.Get(key)
			t.Errorf("extra entry in the transition database: %x -> %x", key, val)
		}
	}
}

func TestSnapshotRandom(t *testing.T) {
	config := &quick.Config{MaxCount: 1000}
	err := quick.Check((*snapshotTest).run, config)
	if cerr, ok := err.(*quick.CheckError); ok {
		test := cerr.In[0].(*snapshotTest)
		t.Errorf("%v:\n%s", test.err, test)
	} else if err != nil {
		t.Error(err)
	}
}

// A snapshotTest checks that reverting StateDB snapshots properly undoes all changes
// captured by the snapshot. Instances of this test with pseudorandom content are created
// by Generate.
//
// The test works as follows:
//
// A new state is created and all actions are applied to it. Several snapshots are taken
// in between actions. The test then reverts each snapshot. For each snapshot the actions
// leading up to it are replayed on a fresh, empty state. The behaviour of all public
// accessor methods on the reverted state must match the return value of the equivalent
// methods on the replayed state.
type snapshotTest struct {
	addrs     []common.Address // all account addresses
	actions   []testAction     // modifications to the state
	snapshots []int            // actions indexes at which snapshot is taken
	err       error            // failure details are reported through this field
}

type testAction struct {
	name   string
	fn     func(testAction, *StateDB)
	args   []int64
	noAddr bool
}

// newTestAction creates a random action that changes state.
func newTestAction(addr common.Address, r *rand.Rand) testAction {
	actions := []testAction{
		{
			name: "SetBalance",
			fn: func(a testAction, s *StateDB) {
				s.SetBalance(addr, big.NewInt(a.args[0]))
			},
			args: make([]int64, 1),
		},
		{
			name: "AddBalance",
			fn: func(a testAction, s *StateDB) {
				s.AddBalance(addr, big.NewInt(a.args[0]))
			},
			args: make([]int64, 1),
		},
		{
			name: "SetNonce",
			fn: func(a testAction, s *StateDB) {
				s.SetNonce(addr, uint64(a.args[0]))
			},
			args: make([]int64, 1),
		},
		{
			name: "SetState",
			fn: func(a testAction, s *StateDB) {
				var key, val common.Hash
				binary.BigEndian.PutUint16(key[:], uint16(a.args[0]))
				binary.BigEndian.PutUint16(val[:], uint16(a.args[1]))
				s.SetState(addr, key, val)
			},
			args: make([]int64, 2),
		},
		{
			name: "SetCode",
			fn: func(a testAction, s *StateDB) {
				code := make([]byte, 16)
				binary.BigEndian.PutUint64(code, uint64(a.args[0]))
				binary.BigEndian.PutUint64(code[8:], uint64(a.args[1]))
				s.SetCode(addr, code)
			},
			args: make([]int64, 2),
		},
		{
			name: "CreateAccount",
			fn: func(a testAction, s *StateDB) {
				s.CreateAccount(addr)
			},
		},
		{
			name: "Suicide",
			fn: func(a testAction, s *StateDB) {
				s.Suicide(addr)
			},
		},
		{
			name: "AddRefund",
			fn: func(a testAction, s *StateDB) {
				s.AddRefund(big.NewInt(a.args[0]).Uint64())
			},
			args:   make([]int64, 1),
			noAddr: true,
		},
		{
			name: "AddLog",
			fn: func(a testAction, s *StateDB) {
				data := make([]byte, 2)
				binary.BigEndian.PutUint16(data, uint16(a.args[0]))
				s.AddLog(&types.Log{Address: addr, Data: data})
			},
			args: make([]int64, 1),
		},
	}
	action := actions[r.Intn(len(actions))]
	var nameargs []string
	if !action.noAddr {
		nameargs = append(nameargs, addr.Hex())
	}
	for _, i := range action.args {
		action.args[i] = rand.Int63n(100)
		nameargs = append(nameargs, fmt.Sprint(action.args[i]))
	}
	action.name += strings.Join(nameargs, ", ")
	return action
}

// Generate returns a new snapshot test of the given size. All randomness is
// derived from r.
func (*snapshotTest) Generate(r *rand.Rand, size int) reflect.Value {
	// Generate random actions.
	addrs := make([]common.Address, 50)
	for i := range addrs {
		addrs[i] = toAddr([]byte{byte(i)})
	}
	actions := make([]testAction, size)
	for i := range actions {
		addr := addrs[r.Intn(len(addrs))]
		actions[i] = newTestAction(addr, r)
	}
	// Generate snapshot indexes.
	nsnapshots := int(math.Sqrt(float64(size)))
	if size > 0 && nsnapshots == 0 {
		nsnapshots = 1
	}
	snapshots := make([]int, nsnapshots)
	snaplen := len(actions) / nsnapshots
	for i := range snapshots {
		// Try to place the snapshots some number of actions apart from each other.
		snapshots[i] = (i * snaplen) + r.Intn(snaplen)
	}
	return reflect.ValueOf(&snapshotTest{addrs, actions, snapshots, nil})
}

func (test *snapshotTest) String() string {
	out := new(bytes.Buffer)
	sindex := 0
	for i, action := range test.actions {
		if len(test.snapshots) > sindex && i == test.snapshots[sindex] {
			fmt.Fprintf(out, "---- snapshot %d ----\n", sindex)
			sindex++
		}
		fmt.Fprintf(out, "%4d: %s\n", i, action.name)
	}
	return out.String()
}

func (test *snapshotTest) run() bool {
	// Run all actions and create snapshots.
	var (
		db, _        = ntsdb.NewMemDatabase()
		state, _     = New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))
		snapshotRevs = make([]int, len(test.snapshots))
		sindex       = 0
	)
	for i, action := range test.actions {
		if len(test.snapshots) > sindex && i == test.snapshots[sindex] {
			snapshotRevs[sindex] = state.Snapshot()
			sindex++
		}
		action.fn(action, state)
	}

	// Revert all snapshots in reverse order. Each revert must yield a state
	// that is equivalent to fresh state with all actions up the snapshot applied.
	for sindex--; sindex >= 0; sindex-- {
		checkstate, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))
		for _, action := range test.actions[:test.snapshots[sindex]] {
			action.fn(action, checkstate)
		}
		state.RevertToSnapshot(snapshotRevs[sindex])
		if err := test.checkEqual(state, checkstate); err != nil {
			test.err = fmt.Errorf("state mismatch after revert to snapshot %d\n%v", sindex, err)
			return false
		}
	}
	return true
}

// checkEqual checks that methods of state and checkstate return the same values.
func (test *snapshotTest) checkEqual(state, checkstate *StateDB) error {
	for _, addr := range test.addrs {
		var err error
		checkeq := func(op string, a, b interface{}) bool {
			if err == nil && !reflect.DeepEqual(a, b) {
				err = fmt.Errorf("got %s(%s) == %v, want %v", op, addr.Hex(), a, b)
				return false
			}
			return true
		}
		// Check basic accessor methods.
		checkeq("Exist", state.Exist(addr), checkstate.Exist(addr))
		checkeq("HasSuicided", state.HasSuicided(addr), checkstate.HasSuicided(addr))
		checkeq("GetBalance", state.GetBalance(addr), checkstate.GetBalance(addr))
		checkeq("GetNonce", state.GetNonce(addr), checkstate.GetNonce(addr))
		checkeq("GetCode", state.GetCode(addr), checkstate.GetCode(addr))
		checkeq("GetCodeHash", state.GetCodeHash(addr), checkstate.GetCodeHash(addr))
		checkeq("GetCodeSize", state.GetCodeSize(addr), checkstate.GetCodeSize(addr))
		// Check storage.
		if obj := state.getStateObject(addr); obj != nil {
			state.ForEachStorage(addr, func(key, val common.Hash) bool {
				return checkeq("GetState("+key.Hex()+")", val, checkstate.GetState(addr, key))
			})
			checkstate.ForEachStorage(addr, func(key, checkval common.Hash) bool {
				return checkeq("GetState("+key.Hex()+")", state.GetState(addr, key), checkval)
			})
		}
		if err != nil {
			return err
		}
	}

	if state.GetRefund() != checkstate.GetRefund() {
		return fmt.Errorf("got GetRefund() == %d, want GetRefund() == %d",
			state.GetRefund(), checkstate.GetRefund())
	}
	if !reflect.DeepEqual(state.GetLogs(common.Hash{}), checkstate.GetLogs(common.Hash{})) {
		return fmt.Errorf("got GetLogs(common.Hash{}) == %v, want GetLogs(common.Hash{}) == %v",
			state.GetLogs(common.Hash{}), checkstate.GetLogs(common.Hash{}))
	}
	return nil
}

func (s *StateSuite) TestTouchDelete(c *check.C) {
	s.state.GetOrNewStateObject(common.Address{})
	root, _ := s.state.CommitTo(s.db)
	s.state.Reset(s.state.dataPrefix, root)

	snapshot := s.state.Snapshot()
	s.state.AddBalance(common.Address{}, new(big.Int))
	if len(s.state.stateObjectsDirty) != 1 {
		c.Fatal("expected one dirty state object")
	}

	s.state.RevertToSnapshot(snapshot)
	if len(s.state.stateObjectsDirty) != 0 {
		c.Fatal("expected no dirty state object")
	}
}

func TestMCState(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	add0 := toAddr([]byte("account0"))
	add1 := toAddr([]byte("account1"))

	storageKey := common.StringToHash("witness")
	data0 := common.StringToHash("a1,a2,a3")
	data1 := common.StringToHash("a4,a5,a6")

	state.SetState(add0, storageKey, data0)
	state.SetState(add1, storageKey, data1)

	so0 := state.getStateObject(add0)
	so0.SetBalance(big.NewInt(42))
	so0.SetNonce(43)

	state.setStateObject(so0)

	state.CommitTo(db)

	got := state.getStateObject(add0)
	t.Log(got.Balance(), got.Nonce(), state.GetState(add0, storageKey).Str())

}
func newAddr(b ...byte) common.Address {
	return common.BytesToAddress(common.LeftPadBytes(b, common.AddressLength))
}

func TestBalanceLog(t *testing.T) {
	t.Skip("skip")
	t.Run("sample", func(t *testing.T) {
		db, _ := ntsdb.NewMemDatabase()
		state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

		addr1, addr2, add3 := newAddr(1), newAddr(2), newAddr(3)
		state.AddBalance(addr1, big.NewInt(100))

		state.AddBalance(addr2, big.NewInt(100))
		state.AddBalance(addr2, big.NewInt(200))
		state.SubBalance(addr2, big.NewInt(50))

		state.AddBalance(add3, big.NewInt(111))
		state.SubBalance(add3, big.NewInt(111))

		_, logs := state.IntermediateRoot(common.Address{})
		want := []BalanceJourna{
			{
				Address: addr1,
				Before:  *big.NewInt(0),
				Now:     *big.NewInt(100),
				Diff:    *big.NewInt(100),
			},
			{
				Address: addr2,
				Before:  *big.NewInt(0),
				Now:     *big.NewInt(100 + 200 - 50),
				Diff:    *big.NewInt(100 + 200 - 50),
			},
			{
				Address: add3,
				Before:  *big.NewInt(0),
				Now:     *big.NewInt(0),
				Diff:    *big.NewInt(0),
			},
		}
		require.Equal(t, want, logs)

		t.Run("changed", func(t *testing.T) {
			root, err := state.CommitTo(db)
			require.NoError(t, err)

			state2, _ := New([]byte{1, 2, 3}, root, NewDatabase(db))
			state2.AddBalance(addr1, big.NewInt(1))
			_, logs := state2.IntermediateRoot(common.Address{})
			want := []BalanceJourna{
				{
					Address: addr1,
					Before:  *big.NewInt(100),
					Now:     *big.NewInt(101),
					Diff:    *big.NewInt(1),
				},
			}
			require.Equal(t, want, logs)
		})
	})

	t.Run("revert", func(t *testing.T) {
		db, _ := ntsdb.NewMemDatabase()
		state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))
		addr1 := newAddr(1)
		state.AddBalance(addr1, big.NewInt(100))

		//创建快照后回退，应该保持不变
		id := state.Snapshot()
		state.AddBalance(addr1, big.NewInt(200))
		state.AddBalance(newAddr(2), big.NewInt(1))
		state.RevertToSnapshot(id)

		_, logs := state.IntermediateRoot(common.Address{})
		require.Len(t, logs, 1)

		want := BalanceJourna{
			Address: addr1,
			Before:  *big.NewInt(0),
			Now:     *big.NewInt(100),
			Diff:    *big.NewInt(100),
		}
		require.Equal(t, want, logs[0])
	})

}

func TestCheckDataPrefix(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	prefixStr := "4b19f83434cff017b19ab0b11ef1988f6e8867e6ab1eb851811bbc3193ba8364"
	prefix := common.FromHex(prefixStr)
	state, err := New(prefix, common.Hash{}, NewDatabase(db))
	require.NoError(t, err)

	var objs []*stateObject
	for i := 1; i <= 3; i++ {
		addr := newAddr(uint8(i))
		state.AddBalance(addr, big.NewInt(int64(i*10)))
		state.SetCode(addr, common.StringToHash(fmt.Sprintf("code_%d", i)).Bytes())
		state.SetState(addr, common.StringToHash("Key1"), common.StringToHash("Value1"))
		state.SetState(addr, common.StringToHash("Key2"), common.StringToHash("Value2"))

		objs = append(objs, state.getStateObject(addr))
	}
	root, err := state.CommitTo(db)
	require.NoError(t, err)

	t.Run("prefix check", func(t *testing.T) {
		//数据库中信息应该都具有前缀
		for _, key := range db.Keys() {
			keyStr := common.Bytes2Hex(key)
			if !strings.HasPrefix(keyStr, prefixStr) {
				require.Fail(t, "missing prefix", "key(%s) missing prefix %s", keyStr, prefixStr)
			}
		}
	})

	//能正常加载数据
	t.Run("load trie", func(t *testing.T) {
		sdb, err := New(prefix, root, NewDatabase(db))
		require.NoError(t, err)
		for _, obj := range objs {

			var keys int
			sdb.ForEachStorage(obj.Address(), func(key, value common.Hash) bool {
				keys++
				return true
			})
			require.Equal(t, 2, keys, "should be have two items")

			code := sdb.GetCode(obj.Address())
			require.Equal(t, []byte(obj.code), code) //能加载 Code
		}

	})
}
