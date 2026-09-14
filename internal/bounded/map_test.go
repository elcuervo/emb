package bounded

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCapAndDeterministicEviction(t *testing.T) {
	m := New[string](2)
	for _, k := range []string{"c", "a", "b"} {
		key := k
		m.GetOrCreate("b", key, func() (string, error) { return key, nil })
	}
	if _, ok := m.Get("b", "a"); ok {
		t.Fatal("a (lexicographically smallest) should have been evicted")
	}
	for _, k := range []string{"b", "c"} {
		if _, ok := m.Get("b", k); !ok {
			t.Fatalf("%q missing after eviction", k)
		}
	}
	if n := m.Len("b"); n != 2 {
		t.Fatalf("Len = %d, want 2", n)
	}
}

func TestExistingKeyDoesNotEvict(t *testing.T) {
	m := New[string](2)
	m.GetOrCreate("b", "a", func() (string, error) { return "a", nil })
	m.GetOrCreate("b", "c", func() (string, error) { return "c", nil })
	// Refreshing an existing key must not trigger the eviction path.
	if _, existed, _ := m.GetOrCreate("b", "a", func() (string, error) { return "a2", nil }); !existed {
		t.Fatal("second insert of a should report existed")
	}
	if v, _ := m.Get("b", "a"); v != "a" {
		t.Fatalf("cached value = %q, want the original a (create must not run)", v)
	}
	if _, ok := m.Get("b", "c"); !ok {
		t.Fatal("c should not have been evicted")
	}
}

func TestBucketsIndependent(t *testing.T) {
	m := New[int](1)
	m.GetOrCreate("x", "k", func() (int, error) { return 1, nil })
	m.GetOrCreate("y", "k", func() (int, error) { return 2, nil })
	if v, _ := m.Get("x", "k"); v != 1 {
		t.Fatalf("x/k = %d, want 1", v)
	}
	if v, _ := m.Get("y", "k"); v != 2 {
		t.Fatalf("y/k = %d, want 2", v)
	}
}

func TestCreateOnceUnderConcurrency(t *testing.T) {
	m := New[string](4)
	var calls atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.GetOrCreate("b", "k", func() (string, error) {
				calls.Add(1)
				return "v", nil
			})
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("create ran %d times, want 1", got)
	}
}

func TestCreateErrorStoresNothing(t *testing.T) {
	m := New[string](4)
	boom := errors.New("boom")
	if _, _, err := m.GetOrCreate("b", "k", func() (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if _, ok := m.Get("b", "k"); ok {
		t.Fatal("failed create must not store a value")
	}
}

func TestClear(t *testing.T) {
	m := New[int](4)
	m.GetOrCreate("x", "k", func() (int, error) { return 1, nil })
	m.GetOrCreate("y", "k", func() (int, error) { return 2, nil })
	m.Clear("x")
	if _, ok := m.Get("x", "k"); ok {
		t.Fatal("x should be cleared")
	}
	if _, ok := m.Get("y", "k"); !ok {
		t.Fatal("y should survive clearing x")
	}
	m.Clear("")
	if m.Len("y") != 0 {
		t.Fatal("Clear(\"\") should drop every bucket")
	}
}
