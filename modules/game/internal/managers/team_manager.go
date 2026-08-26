package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"

	"google.golang.org/protobuf/proto"
)

type TeamManager struct {
	actor.BaseActor
	players *player.Registry
	teams   *team.Registry
}

func NewTeamManager(ctx context.Context, scope *actor.Scope, players *player.Registry, teams *team.Registry) (*TeamManager, error) {
	if players == nil {
		return nil, fmt.Errorf("game: Player Registry is nil")
	}
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	definition, err := actor.Define(scope, actor.Team, func(context.Context, string) (*TeamManager, error) {
		return &TeamManager{players: players, teams: teams}, nil
	})
	if err != nil {
		return nil, err
	}
	return definition.GetOrCreate(ctx, "singleton")
}

func (m *TeamManager) GetTeamByPlayerID(playerID int64) (team.Snapshot, bool) {
	type result struct {
		snapshot team.Snapshot
		found    bool
	}
	response, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (result, error) {
		playerSnapshot, err := m.players.Snapshot(execution, playerID)
		if errors.Is(err, actor.ErrActorStopped) {
			return result{}, nil
		}
		if err != nil {
			return result{}, err
		}
		teamSnapshot, err := m.teams.Snapshot(execution, playerSnapshot.TeamID)
		if errors.Is(err, actor.ErrActorStopped) {
			return result{}, nil
		}
		if err != nil {
			return result{}, err
		}
		return result{snapshot: teamSnapshot, found: true}, nil
	})
	if err != nil {
		log.Error("获取玩家 %d 的队伍失败: %v", playerID, err)
		return team.Snapshot{}, false
	}
	return response.snapshot, response.found
}

func (m *TeamManager) SetRoomProjection(teamID int64, roomID string) bool {
	applied, err := actor.Call(context.Background(), m.Ref(), func(execution actor.Context) (bool, error) {
		applied, err := m.teams.SetRoomProjection(execution, teamID, roomID)
		if errors.Is(err, actor.ErrActorStopped) {
			return false, nil
		}
		return applied, err
	})
	if err != nil {
		log.Error("设置队伍 %d 的房间投影失败: %v", teamID, err)
		return false
	}
	return applied
}

func (m *TeamManager) SendMessage(teamID int64, msg proto.Message) {
	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		return m.sendMessage(execution, teamID, msg, 0, false)
	}); err != nil {
		log.Error("提交队伍 %d 消息失败: %v", teamID, err)
	}
}

func (m *TeamManager) SendMessageExceptSelf(teamID int64, msg proto.Message, selfID int64) {
	if err := actor.Tell(context.Background(), m.Ref(), func(execution actor.Context) error {
		return m.sendMessage(execution, teamID, msg, selfID, true)
	}); err != nil {
		log.Error("提交队伍 %d 排除玩家消息失败: %v", teamID, err)
	}
}

func (m *TeamManager) sendMessage(ctx context.Context, teamID int64, msg proto.Message, excludedPlayerID int64, exclude bool) error {
	snapshot, err := m.teams.Snapshot(ctx, teamID)
	if errors.Is(err, actor.ErrActorStopped) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, memberID := range snapshot.MemberIDs {
		if exclude && memberID == excludedPlayerID {
			continue
		}
		if err := m.players.SendToClient(ctx, memberID, msg); err != nil && !errors.Is(err, actor.ErrActorStopped) {
			log.Error("发送队伍 %d 消息给玩家 %d 失败: %v", teamID, memberID, err)
		}
	}
	return nil
}
