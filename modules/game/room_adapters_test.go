package game

import (
	"context"
	"errors"
	"gameserver/common/msg/message"
	"gameserver/modules/room/participant"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

type projectedTeamRoom struct {
	teamID int64
	roomID string
}

type fakeRoomTeamSetter struct {
	projected []projectedTeamRoom
	fail      map[int64]error
}

func (f *fakeRoomTeamSetter) SetRoomProjection(ctx context.Context, teamID int64, roomID string) error {
	f.projected = append(f.projected, projectedTeamRoom{teamID: teamID, roomID: roomID})
	return f.fail[teamID]
}

func TestRoomTeamProjectionAppliesWholeDesiredBatchAndReportsFailure(t *testing.T) {
	firstFailure := errors.New("team is stopping")
	secondFailure := errors.New("projection unavailable")
	teams := &fakeRoomTeamSetter{fail: map[int64]error{8: firstFailure, 9: secondFailure}}
	projection := NewRoomTeamProjection(teams)

	err := projection.Apply(context.Background(), []participant.DesiredTeamRoom{
		{TeamID: 7, RoomID: "room-1"},
		{TeamID: 8, RoomID: "room-1"},
		{TeamID: 9, RoomID: ""},
	})

	want := []projectedTeamRoom{
		{teamID: 7, roomID: "room-1"},
		{teamID: 8, roomID: "room-1"},
		{teamID: 9, roomID: ""},
	}
	if !errors.Is(err, firstFailure) || !errors.Is(err, secondFailure) || !slices.Equal(teams.projected, want) {
		t.Fatalf("projection = %#v, err = %v", teams.projected, err)
	}
	if !strings.Contains(err.Error(), "team 8") || !strings.Contains(err.Error(), "team 9") {
		t.Fatalf("projection error lost team IDs: %v", err)
	}
}

type sentRoomMessage struct {
	playerID int64
	msg      proto.Message
}

type fakeRoomPlayerSender struct {
	sent []sentRoomMessage
}

func (s *fakeRoomPlayerSender) SendToPlayer(playerID int64, msg proto.Message) {
	s.sent = append(s.sent, sentRoomMessage{playerID: playerID, msg: msg})
}

func TestRoomPlayerMessengerSendsToSelectedPlayers(t *testing.T) {
	sender := &fakeRoomPlayerSender{}
	messenger := NewRoomPlayerMessenger(sender)
	matchResult := &message.S2C_MatchResult{RoomId: "room-1"}
	offline := &message.S2C_PlayerOffline{PlayerId: "42"}

	messenger.Send([]int64{42, 43}, matchResult)
	messenger.SendExcept([]int64{42, 43}, 42, offline)

	sent := sender.sent
	if len(sent) != 3 || sent[0].playerID != 42 || sent[1].playerID != 43 || sent[2].playerID != 43 {
		t.Fatalf("sent messages = %#v", sent)
	}
	if sent[0].msg != matchResult || sent[1].msg != matchResult || sent[2].msg != offline {
		t.Fatalf("message identities = %#v", sent)
	}
}
