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
	players *player.Registry
	teams   *team.Registry
}

func NewTeamManager(players *player.Registry, teams *team.Registry) (*TeamManager, error) {
	if players == nil {
		return nil, fmt.Errorf("game: Player Registry is nil")
	}
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	return &TeamManager{players: players, teams: teams}, nil
}

func (m *TeamManager) GetTeamByPlayerID(ctx context.Context, playerID int64) (team.Snapshot, bool, error) {
	playerSnapshot, err := m.players.Snapshot(ctx, playerID)
	if errors.Is(err, actor.ErrActorNotFound) {
		return team.Snapshot{}, false, nil
	}
	if err != nil {
		return team.Snapshot{}, false, err
	}
	if playerSnapshot.TeamID == 0 {
		return team.Snapshot{}, false, nil
	}
	snapshot, err := m.teams.Snapshot(ctx, playerSnapshot.TeamID)
	if errors.Is(err, actor.ErrActorNotFound) {
		return team.Snapshot{}, false, nil
	}
	return snapshot, err == nil, err
}

func (m *TeamManager) SetRoomProjection(ctx context.Context, teamID int64, roomID string) error {
	return m.teams.SetRoomProjection(ctx, teamID, roomID)
}

func (m *TeamManager) StopBroadcasts(ctx context.Context) error {
	return m.teams.StopBroadcasts(ctx)
}

func (m *TeamManager) SendMessage(teamID int64, msg proto.Message) {
	if err := m.teams.Broadcast(context.Background(), teamID, func(ctx context.Context, memberID int64) error {
		return m.players.SendToClient(ctx, memberID, msg)
	}); err != nil && !errors.Is(err, actor.ErrActorNotFound) {
		log.Error("提交队伍 %d 消息失败: %v", teamID, err)
	}
}

func (m *TeamManager) SendMessageExceptSelf(teamID int64, msg proto.Message, selfID int64) {
	if err := m.teams.Broadcast(context.Background(), teamID, func(ctx context.Context, memberID int64) error {
		if memberID == selfID {
			return nil
		}
		return m.players.SendToClient(ctx, memberID, msg)
	}); err != nil && !errors.Is(err, actor.ErrActorNotFound) {
		log.Error("提交队伍 %d 排除玩家消息失败: %v", teamID, err)
	}
}
