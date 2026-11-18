package game

import (
	"context"
	"errors"
	"fmt"
	"gameserver/modules/room/participant"

	"google.golang.org/protobuf/proto"
)

type roomTeamSetter interface {
	SetRoomProjection(ctx context.Context, teamID int64, roomID string) error
}

type roomTeamProjection struct {
	teams roomTeamSetter
}

func NewRoomTeamProjection(teams roomTeamSetter) participant.TeamRoomProjection {
	return &roomTeamProjection{teams: teams}
}

func (p *roomTeamProjection) Apply(ctx context.Context, desired []participant.DesiredTeamRoom) error {
	var projectionErrors []error
	for _, projection := range desired {
		if err := p.teams.SetRoomProjection(ctx, projection.TeamID, projection.RoomID); err != nil {
			projectionErrors = append(projectionErrors, fmt.Errorf("project RoomID for team %d: %w", projection.TeamID, err))
		}
	}
	return errors.Join(projectionErrors...)
}

type roomPlayerMessenger struct {
	send func(playerID int64, msg proto.Message)
}

type roomPlayerSender interface {
	SendToPlayer(playerID int64, msg proto.Message)
}

func NewRoomPlayerMessenger(players roomPlayerSender) participant.PlayerMessenger {
	return &roomPlayerMessenger{send: players.SendToPlayer}
}

func (m *roomPlayerMessenger) Send(playerIDs []int64, msg proto.Message) {
	for _, playerID := range playerIDs {
		m.send(playerID, msg)
	}
}

func (m *roomPlayerMessenger) SendExcept(playerIDs []int64, excludedPlayerID int64, msg proto.Message) {
	for _, playerID := range playerIDs {
		if playerID != excludedPlayerID {
			m.send(playerID, msg)
		}
	}
}
