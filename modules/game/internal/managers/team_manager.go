package managers

import (
	actor_manager "gameserver/core/actor"
	"gameserver/modules/game/internal/managers/team"
)

type TeamManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
}

func (t *TeamManager) GetTeamByPlayerId(playerId int64) *team.Team {
	player := GetUserManager().DirectCaller.GetPlayer(playerId)
	if player == nil {
		return nil
	}
	return actor_manager.Get[team.Team](player.TeamId)
}

func (t *TeamManager) JoinRoom(playerId int64, roomId int64) {
	player := GetUserManager().DirectCaller.GetPlayer(playerId)
	if player == nil {
		return
	}
	team.JoinRoom(player.TeamId, roomId)
}

func (t *TeamManager) LeaveRoom(teamId int64) {
	team := actor_manager.Get[team.Team](teamId)
	if team == nil {
		return
	}
	team.LeaveRoom()
}
