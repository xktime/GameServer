package actor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLookupPreservesCancellationForMissingAndReadyActors(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "lookup-cancellation")
	if _, err := definition.GetOrCreate(context.Background(), "ready"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, id := range []string{"missing", "ready"} {
		if _, err := definition.Lookup(ctx, id); !errors.Is(err, context.Canceled) {
			t.Fatalf("Lookup(%q) = %v; want context.Canceled", id, err)
		}
	}
}

func TestRebuildQuarantinedValidatesPreviousAndRetainsCanceledCleanup(t *testing.T) {
	system := NewActorSystem(time.Second)
	t.Cleanup(func() { _ = system.Stop(context.Background()) })
	definition := newQuarantineDefinition(t, system, "rebuild-quarantine")
	previous, _ := definition.GetOrCreate(context.Background(), "singleton")
	cleanupCalls := 0
	cleanup := func(context.Context) error { cleanupCalls++; return nil }
	if _, err := definition.RebuildQuarantined(context.Background(), "singleton", previous.Ref(), cleanup); !errors.Is(err, ErrActorNotQuarantined) || cleanupCalls != 0 {
		t.Fatalf("healthy rebuild = %v; cleanup calls = %d", err, cleanupCalls)
	}
	if _, err := definition.RebuildQuarantined(context.Background(), "other", previous.Ref(), cleanup); !errors.Is(err, ErrStaleActorRef) || cleanupCalls != 0 {
		t.Fatalf("foreign rebuild = %v; cleanup calls = %d", err, cleanupCalls)
	}
	previous, target := quarantineTestInstance(t, definition, "singleton")
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := definition.RebuildQuarantined(ctx, "singleton", previous.Ref(), func(context.Context) error { cancel(); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cleanup = %v", err)
	}
	if retained, err := definition.QuarantineTarget(context.Background(), "singleton"); err != nil || retained != target {
		t.Fatalf("canceled rebuild changed the quarantined generation: %v, %v", retained, err)
	}
	replacement, err := definition.RebuildQuarantined(context.Background(), "singleton", previous.Ref(), cleanup)
	if err != nil || replacement == previous || cleanupCalls != 1 {
		t.Fatalf("rebuild = %v; cleanup calls = %d", err, cleanupCalls)
	}
	// If domain initialization fails before publication, the old handle can
	// retry initialization on the replacement without cleaning it a second time.
	retried, err := definition.RebuildQuarantined(context.Background(), "singleton", previous.Ref(), cleanup)
	if err != nil || retried != replacement || cleanupCalls != 1 {
		t.Fatalf("initialization retry = %v; cleanup calls = %d", err, cleanupCalls)
	}
}
