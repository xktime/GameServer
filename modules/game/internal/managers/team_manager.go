package managers

import (
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"

	"google.golang.org/protobuf/proto"
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

func (t *TeamManager) SendMessage(teamId int64, msg proto.Message) {
	team := actor_manager.Get[team.Team](teamId)
	if team == nil {
		return
	}
	for _, member := range team.TeamMembers {
		p := GetUserManager().DirectCaller.GetPlayer(member)
		if p == nil {
			log.Debug("玩家 %d 不在线", member)
			continue
		}
		p.SendToClient(msg)
	}
}
