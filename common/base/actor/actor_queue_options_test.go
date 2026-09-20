package actor

import (
	"context"
	"testing"
	"time"
)

type queueDefaultTestActor struct {
	BaseActor
}

type queueManagerTestActor struct {
	BaseActor
}

func TestDefineUsesGroupQueueDefaults(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("queue-defaults")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	playerDefinition, err := Define(scope, Player, func(context.Context, int64) (*queueDefaultTestActor, error) {
		return &queueDefaultTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define player: %v", err)
	}
	managerDefinition, err := Define(scope, User, func(context.Context, string) (*queueManagerTestActor, error) {
		return &queueManagerTestActor{}, nil
	})
	if err != nil {
		t.Fatalf("define manager: %v", err)
	}
	player, err := playerDefinition.GetOrCreate(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("create player: %v", err)
	}
	manager, err := managerDefinition.GetOrCreate(context.Background(), "singleton")
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	playerEntry, err := player.Ref().resolve()
	if err != nil {
		t.Fatalf("resolve player: %v", err)
	}
	managerEntry, err := manager.Ref().resolve()
	if err != nil {
		t.Fatalf("resolve manager: %v", err)
	}
	if got := cap(playerEntry.cell.queue); got != defaultEntityQueueCapacity {
		t.Fatalf("player queue capacity = %d, want %d", got, defaultEntityQueueCapacity)
	}
	if got := cap(managerEntry.cell.queue); got != defaultManagerQueueCapacity {
		t.Fatalf("manager queue capacity = %d, want %d", got, defaultManagerQueueCapacity)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("stop system: %v", err)
	}
}

func TestDefineWithOptionsOverridesQueueDefault(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("queue-override")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := DefineWithOptions(scope, Player, func(context.Context, int64) (*queueDefaultTestActor, error) {
		return &queueDefaultTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 3})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	entry, err := instance.Ref().resolve()
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if got := cap(entry.cell.queue); got != 3 {
		t.Fatalf("queue capacity = %d, want 3", got)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("stop system: %v", err)
	}
}
