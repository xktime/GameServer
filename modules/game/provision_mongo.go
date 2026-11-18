package game

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	"gameserver/modules/game/internal/provision"
)

type mongoProvisionMaterializer struct{}

func (mongoProvisionMaterializer) Apply(ctx context.Context, intent provision.Intent) error {
	teamDoc := &team.Team{
		TeamId:      intent.Team.TeamID,
		LeaderId:    intent.Team.LeaderID,
		TeamMembers: append([]int64(nil), intent.Team.MemberIDs...),
		RoomId:      intent.Team.RoomID,
	}
	if _, err := mongodb.SaveIfAbsentContext(ctx, teamDoc); err != nil {
		return fmt.Errorf("save Team %d: %w", intent.Team.TeamID, err)
	}

	playerDoc := &player.Player{
		PlayerId:   intent.Player.PlayerID,
		PlayerInfo: clonePlayerInfo(intent.Player.PlayerInfo),
		TeamId:     intent.Player.TeamID,
		TowerLevel: intent.Player.TowerLevel,
	}
	if _, err := mongodb.SaveIfAbsentContext(ctx, playerDoc); err != nil {
		return fmt.Errorf("save Player %d: %w", intent.Player.PlayerID, err)
	}

	if _, err := mongodb.SaveIfAbsentContext(ctx, &intent.User); err != nil {
		return fmt.Errorf("save User %s: %w", intent.User.AccountId, err)
	}
	return nil
}

func clonePlayerInfo(info *playermodel.PlayerInfo) *playermodel.PlayerInfo {
	if info == nil {
		return nil
	}
	copyOfInfo := *info
	return &copyOfInfo
}
