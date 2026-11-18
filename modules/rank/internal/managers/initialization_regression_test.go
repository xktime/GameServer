package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"sync"
	"testing"
	"time"
)

type blockingInitializationRankStore struct {
	memoryRankStore
	entered chan context.Context
	release chan struct{}
}

func (s *blockingInitializationRankStore) LoadSeason(ctx context.Context) (*SeasonManager, error) {
	s.entered <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return nil, nil
	}
}

func TestRankManagerInitializationHonorsCancellationAndDeadline(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancellation"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			system := actor.NewActorSystem(time.Second)
			scope, err := system.NewScope("rank-initialization")
			if err != nil {
				t.Fatal(err)
			}
			store := &blockingInitializationRankStore{entered: make(chan context.Context, 1), release: make(chan struct{})}
			release := sync.OnceFunc(func() { close(store.release) })
			timeout := 5 * time.Second
			if deadline {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			done := make(chan struct{})
			var initializationError error
			go func() {
				_, initializationError = newRankManager(ctx, scope, fakePlayerReader{}, store, rankTestNow)
				close(done)
			}()
			t.Cleanup(func() { cancel(); release(); <-done; _ = system.Stop(context.Background()) })
			select {
			case observed := <-store.entered:
				want, _ := ctx.Deadline()
				if got, ok := observed.Deadline(); !ok || !got.Equal(want) {
					t.Errorf("storage deadline = %v, %t; want %v", got, ok, want)
				}
			case <-time.After(time.Second):
				t.Fatal("initialization did not enter storage")
			}
			if !deadline {
				cancel()
			}
			<-ctx.Done()
			select {
			case <-done:
				if !errors.Is(initializationError, ctx.Err()) {
					t.Fatalf("initialization = %v; want %v", initializationError, ctx.Err())
				}
			case <-time.After(time.Second):
				t.Fatal("initialization remained blocked after its context expired")
			}
		})
	}
}
