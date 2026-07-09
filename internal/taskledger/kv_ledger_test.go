package taskledger_test

import (
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

type fakeKVStore struct {
	mu    sync.Mutex
	items map[string]fakeKVItem
}

type fakeKVItem struct {
	value    []byte
	revision uint64
	created  time.Time
}

type fakeKV struct {
	store *fakeKVStore
}

func newFakeKV() *fakeKV {
	return &fakeKV{store: &fakeKVStore{items: make(map[string]fakeKVItem)}}
}

type fakeKVEntry struct {
	key      string
	value    []byte
	revision uint64
	created  time.Time
}

func (e fakeKVEntry) Bucket() string             { return "test" }
func (e fakeKVEntry) Key() string                { return e.key }
func (e fakeKVEntry) Value() []byte              { return append([]byte(nil), e.value...) }
func (e fakeKVEntry) Revision() uint64           { return e.revision }
func (e fakeKVEntry) Created() time.Time         { return e.created }
func (e fakeKVEntry) Delta() uint64              { return 0 }
func (e fakeKVEntry) Operation() nats.KeyValueOp { return nats.KeyValuePut }

func (f *fakeKV) Get(key string) (nats.KeyValueEntry, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	item, ok := f.store.items[key]
	if !ok {
		return nil, nats.ErrKeyNotFound
	}
	return fakeKVEntry{key: key, value: item.value, revision: item.revision, created: item.created}, nil
}

func (f *fakeKV) Create(key string, value []byte) (uint64, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	if _, ok := f.store.items[key]; ok {
		return 0, nats.ErrKeyExists
	}
	rev := uint64(1)
	f.store.items[key] = fakeKVItem{value: append([]byte(nil), value...), revision: rev, created: time.Now().UTC()}
	return rev, nil
}

func (f *fakeKV) Update(key string, value []byte, last uint64) (uint64, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	item, ok := f.store.items[key]
	if !ok {
		return 0, nats.ErrKeyNotFound
	}
	if item.revision != last {
		return 0, nats.ErrKeyExists
	}
	item.revision++
	item.value = append([]byte(nil), value...)
	f.store.items[key] = item
	return item.revision, nil
}

func (f *fakeKV) Put(key string, value []byte) (uint64, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	item, ok := f.store.items[key]
	if !ok {
		rev := uint64(1)
		f.store.items[key] = fakeKVItem{value: append([]byte(nil), value...), revision: rev, created: time.Now().UTC()}
		return rev, nil
	}
	item.revision++
	item.value = append([]byte(nil), value...)
	f.store.items[key] = item
	return item.revision, nil
}

func (f *fakeKV) GetRevision(string, uint64) (nats.KeyValueEntry, error) {
	return nil, nats.ErrKeyNotFound
}
func (f *fakeKV) PutString(string, string) (uint64, error) { return 0, errors.New("not implemented") }
func (f *fakeKV) Delete(string, ...nats.DeleteOpt) error   { return errors.New("not implemented") }
func (f *fakeKV) Purge(string, ...nats.DeleteOpt) error    { return errors.New("not implemented") }
func (f *fakeKV) Watch(string, ...nats.WatchOpt) (nats.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeKV) WatchAll(...nats.WatchOpt) (nats.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeKV) WatchFiltered([]string, ...nats.WatchOpt) (nats.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeKV) Keys(...nats.WatchOpt) ([]string, error) { return nil, errors.New("not implemented") }
func (f *fakeKV) ListKeys(...nats.WatchOpt) (nats.KeyLister, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeKV) History(string, ...nats.WatchOpt) ([]nats.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeKV) Bucket() string                       { return "test" }
func (f *fakeKV) PurgeDeletes(...nats.PurgeOpt) error  { return errors.New("not implemented") }
func (f *fakeKV) Status() (nats.KeyValueStatus, error) { return nil, errors.New("not implemented") }

func mustConsumeEvent(t *testing.T, traceID string, amount int) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.EnergyConsumePayload{
		Kind:   protocol.SignalEnergyConsume,
		Amount: amount,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestKVLedgerConcurrentConsumeOnlyOneSucceeds(t *testing.T) {
	kv := newFakeKV()
	ledger := taskledger.NewKVLedger(kv)

	seed := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    1,
		EnergyRemaining: 1,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Create("trace-1", data); err != nil {
		t.Fatal(err)
	}

	var okCount atomic.Int32
	var failCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ledger.Apply(mustConsumeEvent(t, "trace-1", 1))
			if err != nil {
				failCount.Add(1)
				return
			}
			okCount.Add(1)
		}()
	}
	wg.Wait()

	if okCount.Load() != 1 {
		t.Fatalf("successful consumes = %d, want 1", okCount.Load())
	}
	if failCount.Load() != 1 {
		t.Fatalf("failed consumes = %d, want 1", failCount.Load())
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 0 {
		t.Fatalf("remaining = %d, want 0", snap.EnergyRemaining)
	}
}

func TestKVLedgerSeedEnergyPreservesPriorAdd(t *testing.T) {
	kv := newFakeKV()
	ledger := taskledger.NewKVLedger(kv)

	add, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", snap.EnergyRemaining)
	}
	if snap.EnergyBudget != 12 {
		t.Fatalf("budget = %d, want 12", snap.EnergyBudget)
	}
}

func TestEnsureEnergySeededPreservesPriorAdd(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyRemaining: 5,
	}
	updated, changed := taskledger.EnsureEnergySeeded(trace, 12)
	if !changed {
		t.Fatal("expected changed")
	}
	if updated.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d, want 5", updated.EnergyRemaining)
	}
	if updated.EnergyBudget != 12 {
		t.Fatalf("budget = %d, want 12", updated.EnergyBudget)
	}
}

func TestApplyEventEnergyAddBeforeSeedLeavesBudgetUnset(t *testing.T) {
	trace := taskledger.TraceSnapshot{TraceID: "trace-1", Tasks: map[string]taskledger.TaskSnapshot{}}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyRemaining != 5 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
	if res.Trace.EnergyBudget != 0 {
		t.Fatalf("budget = %d, want 0 so SeedEnergy can apply colony defaults", res.Trace.EnergyBudget)
	}
}
