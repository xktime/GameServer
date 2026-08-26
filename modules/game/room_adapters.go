package game

import (
	"fmt"
	"gameserver/modules/game/internal/managers"
	"gameserver/modules/room/participant"

	"google.golang.org/protobuf/proto"
)

type roomTeamSetter interface {
	SetRoomProjection(teamID int64, roomID string) bool
}

type roomTeamProjection struct {
	teams roomTeamSetter
}

func NewRoomTeamProjection(teams *managers.TeamManager) participant.TeamRoomProjection {
	return newRoomTeamProjection(teams)
}

func newRoomTeamProjection(teams roomTeamSetter) participant.TeamRoomProjection {
	return &roomTeamProjection{teams: teams}
}

func (p *roomTeamProjection) Apply(desired []participant.DesiredTeamRoom) error {
	failedTeamIDs := make([]int64, 0)
	for _, projection := range desired {
		if !p.teams.SetRoomProjection(projection.TeamID, projection.RoomID) {
			failedTeamIDs = append(failedTeamIDs, projection.TeamID)
		}
	}
	if len(failedTeamIDs) > 0 {
		return fmt.Errorf("failed to project RoomID for teams %v", failedTeamIDs)
	}
	return nil
}

type roomPlayerMessenger struct {
	send func(playerID int64, msg proto.Message)
}

type roomPlayerSender interface {
	SendToPlayer(playerID int64, msg proto.Message)
}

func NewRoomPlayerMessenger(players roomPlayerSender) participant.PlayerMessenger {
	return newRoomPlayerMessenger(players.SendToPlayer)
}

func newRoomPlayerMessenger(send func(playerID int64, msg proto.Message)) participant.PlayerMessenger {
	return &roomPlayerMessenger{send: send}
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
