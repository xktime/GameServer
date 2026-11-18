package utils

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestKeyedMutexSerializesSameKey(t *testing.T) {
	var mutex KeyedMutex[int]
	var workers sync.WaitGroup
	value := 0
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				unlock, err := mutex.Lock(context.Background(), 7)
				if err != nil {
					t.Error(err)
					return
				}
				value++
				unlock()
			}
		}()
	}
	workers.Wait()
	if value != 1600 {
		t.Fatalf("serialized increments = %d; want 1600", value)
	}
	if len(mutex.entries) != 0 {
		t.Fatalf("released keys retained = %d", len(mutex.entries))
	}
}

func TestKeyedMutexAllowsDifferentKeysAndCancelsWaiters(t *testing.T) {
	var mutex KeyedMutex[int]
	unlock, err := mutex.Lock(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	otherUnlock, err := mutex.Lock(ctx, 8)
	if err != nil {
		t.Fatalf("different key blocked: %v", err)
	}
	otherUnlock()

	waitContext, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	if _, err := mutex.Lock(waitContext, 7); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting on held key = %v", err)
	}
	mutex.mu.Lock()
	defer mutex.mu.Unlock()
	if len(mutex.entries) != 1 || mutex.entries[7].users != 1 {
		t.Fatalf("canceled waiter retained: %+v", mutex.entries)
	}
}

func TestKeyedMutexCanceledContextDoesNotAcquireOrRetainKey(t *testing.T) {
	var mutex KeyedMutex[string]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mutex.Lock(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquisition = %v", err)
	}
	if len(mutex.entries) != 0 {
		t.Fatalf("canceled acquisition retained keys: %v", mutex.entries)
	}
	for _, key := range []string{"a", "b", "c"} {
		unlock, err := mutex.Lock(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		unlock()
	}
	if len(mutex.entries) != 0 {
		t.Fatalf("released keys retained = %d", len(mutex.entries))
	}
}
