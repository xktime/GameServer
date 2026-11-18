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

func TestDefinitionsDeclareQueueCapacity(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("queue-defaults")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	playerDefinition, err := Define(scope, func(context.Context, int64) (*queueDefaultTestActor, error) {
		return &queueDefaultTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 256})
	if err != nil {
		t.Fatalf("define player: %v", err)
	}
	managerDefinition, err := Define(scope, func(context.Context, string) (*queueManagerTestActor, error) {
		return &queueManagerTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 1024})
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

	playerCell, err := player.Ref().resolveCell()
	if err != nil {
		t.Fatalf("resolve player: %v", err)
	}
	managerCell, err := manager.Ref().resolveCell()
	if err != nil {
		t.Fatalf("resolve manager: %v", err)
	}
	if got := cap(playerCell.queue); got != 256 {
		t.Fatalf("player queue capacity = %d, want 256", got)
	}
	if got := cap(managerCell.queue); got != 1024 {
		t.Fatalf("manager queue capacity = %d, want 1024", got)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("stop system: %v", err)
	}
}

func TestDefinitionOptionsOverrideQueueDefault(t *testing.T) {
	system := NewActorSystem(time.Second)
	scope, err := system.NewScope("queue-override")
	if err != nil {
		t.Fatalf("create scope: %v", err)
	}
	definition, err := Define(scope, func(context.Context, int64) (*queueDefaultTestActor, error) {
		return &queueDefaultTestActor{}, nil
	}, DefinitionOptions{QueueCapacity: 3})
	if err != nil {
		t.Fatalf("define actor: %v", err)
	}
	instance, err := definition.GetOrCreate(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	cell, err := instance.Ref().resolveCell()
	if err != nil {
		t.Fatalf("resolve actor: %v", err)
	}
	if got := cap(cell.queue); got != 3 {
		t.Fatalf("queue capacity = %d, want 3", got)
	}
	if err := system.Stop(context.Background()); err != nil {
		t.Fatalf("stop system: %v", err)
	}
}
