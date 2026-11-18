package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"testing"
	"time"
)

func TestConnectRecoveryRebindsTheExistingManager(t *testing.T) {
	manager := newConnectManagerTestHarness(t, time.Now)
	previous := manager.currentActor()
	for range 3 {
		_, err := actor.Command(context.Background(), previous.Ref(), func(actor.Context) (struct{}, error) { panic("quarantine") })
		if !errors.Is(err, actor.ErrHandlerPanic) {
			t.Fatal(err)
		}
	}
	if err := manager.Recover(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if manager.currentActor() == previous || manager.currentActor().sessions != previous.sessions {
		t.Fatal("recovery lost the shared login state or retained the old actor")
	}
	manager.Register(&testConnectionAgent{id: 42})
	if manager.GetActiveClients() != 1 {
		t.Fatal("existing Manager handle did not register a connection after recovery")
	}
}
