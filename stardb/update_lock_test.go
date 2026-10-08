package stardb

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// overlapStore fails an update that starts while another one on the same key
// is still running, the way a compare-and-swap backend reports a conflict.
type overlapStore struct {
	Store
	running atomic.Int32
	value   int
}

func (s *overlapStore) update(_ context.Context, _ string, dst any, reset func(bool) error, change func() error) error {
	if s.running.Add(1) > 1 {
		s.running.Add(-1)
		return ErrConflict
	}
	defer s.running.Add(-1)
	if err := reset(true); err != nil {
		return err
	}
	*dst.(*int) = s.value
	time.Sleep(time.Millisecond)
	if err := change(); err != nil {
		return err
	}
	s.value = *dst.(*int)
	return nil
}

func TestUpdatesToOneKeyDoNotOverlapInProcess(t *testing.T) {
	store := &overlapStore{}
	const workers = 50
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			if _, err := Update(context.Background(), store, "count", 0, func(n *int) error {
				*n++
				return nil
			}); err != nil {
				t.Errorf("Update: %v", err)
			}
		})
	}
	group.Wait()
	if store.value != workers {
		t.Fatalf("value = %d, want %d", store.value, workers)
	}
	updateLocks.Lock()
	left := len(updateLocks.m)
	updateLocks.Unlock()
	if left != 0 {
		t.Fatalf("%d update locks were not released", left)
	}
}
