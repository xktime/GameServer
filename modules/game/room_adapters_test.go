package game

import (
	"gameserver/common/msg/message"
	"gameserver/modules/room/participant"
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"
)

type projectedTeamRoom struct {
	teamID int64
	roomID string
}

type fakeRoomTeamSetter struct {
	projected []projectedTeamRoom
	fail      map[int64]bool
}

func (f *fakeRoomTeamSetter) SetRoomProjection(teamID int64, roomID string) bool {
	f.projected = append(f.projected, projectedTeamRoom{teamID: teamID, roomID: roomID})
	return !f.fail[teamID]
}

func TestRoomTeamProjectionAppliesWholeDesiredBatchAndReportsFailure(t *testing.T) {
	teams := &fakeRoomTeamSetter{fail: map[int64]bool{8: true}}
	projection := newRoomTeamProjection(teams)

	err := projection.Apply([]participant.DesiredTeamRoom{
		{TeamID: 7, RoomID: "room-1"},
		{TeamID: 8, RoomID: "room-1"},
		{TeamID: 9, RoomID: ""},
	})

	want := []projectedTeamRoom{
		{teamID: 7, roomID: "room-1"},
		{teamID: 8, roomID: "room-1"},
		{teamID: 9, roomID: ""},
	}
	if err == nil || !slices.Equal(teams.projected, want) {
		t.Fatalf("projection = %#v, err = %v", teams.projected, err)
	}
}

type sentRoomMessage struct {
	playerID int64
	msg      proto.Message
}

func TestRoomPlayerMessengerSendsToSelectedPlayers(t *testing.T) {
	var sent []sentRoomMessage
	messenger := newRoomPlayerMessenger(func(playerID int64, msg proto.Message) {
		sent = append(sent, sentRoomMessage{playerID: playerID, msg: msg})
	})
	matchResult := &message.S2C_MatchResult{RoomId: "room-1"}
	offline := &message.S2C_PlayerOffline{PlayerId: "42"}

	messenger.Send([]int64{42, 43}, matchResult)
	messenger.SendExcept([]int64{42, 43}, 42, offline)

	if len(sent) != 3 || sent[0].playerID != 42 || sent[1].playerID != 43 || sent[2].playerID != 43 {
		t.Fatalf("sent messages = %#v", sent)
	}
	if sent[0].msg != matchResult || sent[1].msg != matchResult || sent[2].msg != offline {
		t.Fatalf("message identities = %#v", sent)
	}
}
