package actor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type admissionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *admissionWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestStopWakesBlockedSendersAndRespectsDeadline(t *testing.T) {
	senders := []struct {
		name string
		send func(context.Context, ActorRef, func(Context) error) error
	}{
		{name: "Tell", send: Tell},
		{name: "Call", send: func(ctx context.Context, ref ActorRef, task func(Context) error) error {
			_, err := Call(ctx, ref, func(execution Context) (struct{}, error) {
				return struct{}{}, task(execution)
			})
			return err
		}},
		{name: "CallUntilDone", send: func(ctx context.Context, ref ActorRef, task func(Context) error) error {
			_, err := CallUntilDone(ctx, ref, func(execution Context) (struct{}, error) {
				return struct{}{}, task(execution)
			})
			return err
		}},
	}
	for _, stop := range []struct {
		name  string
		stop  func(ActorRef, context.Context) error
		force bool
	}{
		{name: "Stop", stop: ActorRef.Stop},
		{name: "ForceStop", stop: ActorRef.ForceStop, force: true},
	} {
		for _, sender := range senders {
			t.Run(stop.name+"/"+sender.name, func(t *testing.T) {
				system := NewActorSystem(0)
				scope, err := system.NewScope("blocked-admission")
				if err != nil {
					t.Fatal(err)
				}
				definition, err := DefineWithOptions(scope, Test1, func(context.Context, int) (*systemTestActor, error) {
					return &systemTestActor{}, nil
				}, DefinitionOptions{QueueCapacity: 1})
				if err != nil {
					t.Fatal(err)
				}
				instance, err := definition.GetOrCreate(context.Background(), 1)
				if err != nil {
					t.Fatal(err)
				}
				started := make(chan struct{})
				release := make(chan struct{})
				var releaseOnce sync.Once
				senderContext, cancelSender := context.WithCancel(context.Background())
				t.Cleanup(func() {
					cancelSender()
					releaseOnce.Do(func() { close(release) })
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					if err := system.Stop(ctx); err != nil {
						t.Errorf("cleanup system: %v", err)
					}
				})
				if err := Tell(context.Background(), instance.Ref(), func(Context) error {
					close(started)
					<-release
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				<-started
				var queuedRan atomic.Bool
				if err := TryTell(instance.Ref(), func(Context) error {
					queuedRan.Store(true)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				waitingContext := &admissionWaitContext{Context: senderContext, waiting: make(chan struct{})}
				senderDone := make(chan error, 1)
				var rejectedRan atomic.Bool
				go func() {
					senderDone <- sender.send(waitingContext, instance.Ref(), func(Context) error {
						rejectedRan.Store(true)
						return nil
					})
				}()
				select {
				case <-waitingContext.waiting:
				case <-time.After(time.Second):
					t.Fatal("sender did not start waiting for admission")
				}
				stopContext, cancelStop := context.WithTimeout(context.Background(), 30*time.Millisecond)
				defer cancelStop()
				stopDone := make(chan error, 1)
				go func() { stopDone <- stop.stop(instance.Ref(), stopContext) }()
				select {
				case err := <-stopDone:
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("stop with blocked handler = %v; want deadline exceeded", err)
					}
				case <-time.After(300 * time.Millisecond):
					t.Fatal("stop deadline was blocked by a sender waiting for admission")
				}
				select {
				case err := <-senderDone:
					if !errors.Is(err, ErrActorStopped) {
						t.Fatalf("blocked sender = %v; want ErrActorStopped", err)
					}
				case <-time.After(time.Second):
					t.Fatal("stop did not wake the blocked sender")
				}
				if err := TryTell(instance.Ref(), func(Context) error { return nil }); !errors.Is(err, ErrActorStopped) {
					t.Fatalf("new sender after stop = %v; want ErrActorStopped", err)
				}
				releaseOnce.Do(func() { close(release) })
				finishContext, cancelFinish := context.WithTimeout(context.Background(), time.Second)
				defer cancelFinish()
				if err := instance.Ref().Stop(finishContext); err != nil {
					t.Fatalf("wait for stopped actor: %v", err)
				}
				if got, want := queuedRan.Load(), !stop.force; got != want {
					t.Fatalf("queued task ran = %t; want %t", got, want)
				}
				if rejectedRan.Load() {
					t.Fatal("rejected task was executed")
				}
			})
		}
	}
}

func TestConcurrentAdmissionAndStopPreserveAcceptedTasks(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	scope, err := system.NewScope("concurrent-stop")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := DefineWithOptions(scope, Test1, func(context.Context, int) (*systemTestActor, error) {
		return &systemTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	for id := range 64 {
		instance, err := definition.GetOrCreate(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		const senders = 16
		start := make(chan struct{})
		results := make(chan error, senders)
		var executed atomic.Int32
		for range senders {
			go func() {
				<-start
				results <- Tell(context.Background(), instance.Ref(), func(Context) error {
					executed.Add(1)
					return nil
				})
			}()
		}
		stopped := make(chan error, 1)
		stopContext, cancelStop := context.WithTimeout(context.Background(), time.Second)
		go func() {
			<-start
			stopped <- instance.Ref().Stop(stopContext)
		}()
		close(start)
		var accepted int32
		for range senders {
			err := <-results
			if err == nil {
				accepted++
			} else if !errors.Is(err, ErrActorStopped) {
				cancelStop()
				t.Fatalf("iteration %d: send = %v", id, err)
			}
		}
		err = <-stopped
		cancelStop()
		if err != nil {
			t.Fatalf("iteration %d: stop = %v", id, err)
		}
		if got := executed.Load(); got != accepted {
			t.Fatalf("iteration %d: executed %d tasks; accepted %d", id, got, accepted)
		}
	}
}
