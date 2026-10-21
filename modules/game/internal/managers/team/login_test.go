package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"slices"
	"testing"
)

func TestAbortLoginDoesNotDeleteDurableTeam(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)
	prepared, err := registry.PrepareLogin(context.Background(), 7, 42, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.lookup(context.Background(), 7); !errors.Is(err, actor.ErrActorStopped) {
		t.Fatalf("temporary Team remained: %v", err)
	}
	if len(repository.deletedIDs) != 0 || len(repository.saved) != 0 {
		t.Fatal("abort mutated durable Team")
	}
}

func TestAbortLoginKeepsOtherMembersAndExistingMembership(t *testing.T) {
	repository := &fakeTeamRepository{}
	registry := newTeamRepositoryTestRegistry(t, repository)
	prepared, err := registry.PrepareLogin(context.Background(), 7, 42, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Join(context.Background(), 7, 43); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Snapshot(context.Background(), 7)
	if err != nil || !slices.Equal(snapshot.MemberIDs, []int64{43}) || snapshot.LeaderID != 43 {
		t.Fatalf("other member lost: %#v %v", snapshot, err)
	}
	existing, err := registry.PrepareLogin(context.Background(), 7, 43, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := existing.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, err = registry.Snapshot(context.Background(), 7)
	if err != nil || !slices.Equal(snapshot.MemberIDs, []int64{43}) {
		t.Fatalf("existing membership lost: %#v %v", snapshot, err)
	}
}
