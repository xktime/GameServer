package team

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/db/mongodb"
	"gameserver/common/utils"
	"gameserver/core/log"
	"slices"
)

type Team struct {
	actor.BaseActor `bson:"-"`
	TeamId          int64   `bson:"_id"`
	LeaderId        int64   `bson:"leader_id"`
	TeamMembers     []int64 `bson:"team_members"`
	RoomId          string  `bson:"room_id"`
	deleted         bool    `bson:"-"`
}

type Snapshot struct {
	TeamID    int64
	LeaderID  int64
	MemberIDs []int64
	RoomID    string
}

type Registry struct {
	definition *actor.Definition[*Team, int64]
}

func NewRegistry(scope *actor.Scope) (*Registry, error) {
	definition, err := actor.Define(scope, actor.Team, func(context.Context, int64) (*Team, error) {
		return &Team{}, nil
	})
	if err != nil {
		return nil, err
	}
	return &Registry{definition: definition}, nil
}

func (r *Registry) Create(ctx context.Context, leaderID int64) (Snapshot, error) {
	return r.GetOrCreate(ctx, utils.FlakeId(), leaderID)
}

func (r *Registry) GetOrCreate(ctx context.Context, teamID int64, leaderID int64) (Snapshot, error) {
	team, err := r.definition.GetOrCreate(ctx, teamID)
	if err != nil {
		return Snapshot{}, err
	}
	return actor.Call(ctx, team.Ref(), func(actor.Context) (Snapshot, error) {
		if team.TeamId == 0 {
			team.TeamId = teamID
			team.LeaderId = leaderID
		}
		return team.snapshot(), nil
	})
}

func (r *Registry) lookup(ctx context.Context, teamID int64) (*Team, error) {
	return r.definition.Lookup(ctx, teamID)
}

func (r *Registry) Snapshot(ctx context.Context, teamID int64) (Snapshot, error) {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return Snapshot{}, err
	}
	return actor.Call(ctx, team.Ref(), func(actor.Context) (Snapshot, error) {
		return team.snapshot(), nil
	})
}

func (r *Registry) Join(ctx context.Context, teamID int64, playerID int64) error {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return err
	}
	_, err = actor.Call(ctx, team.Ref(), func(actor.Context) (struct{}, error) {
		if slices.Contains(team.TeamMembers, playerID) {
			return struct{}{}, nil
		}
		if team.LeaderId == 0 {
			team.LeaderId = playerID
		}
		team.TeamMembers = append(team.TeamMembers, playerID)
		log.Debug("玩家 %d 成功加入队伍 %d，当前成员数量: %d", playerID, team.TeamId, len(team.TeamMembers))
		return struct{}{}, nil
	})
	return err
}

func (r *Registry) Leave(ctx context.Context, teamID int64, playerID int64) error {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return err
	}
	empty, err := actor.Call(ctx, team.Ref(), func(actor.Context) (bool, error) {
		log.Debug("玩家 %d 请求离开队伍 %d", playerID, team.TeamId)
		if team.LeaderId == playerID {
			team.LeaderId = 0
		}
		for index, memberID := range team.TeamMembers {
			if memberID == playerID {
				team.TeamMembers = append(team.TeamMembers[:index], team.TeamMembers[index+1:]...)
				break
			}
		}
		if len(team.TeamMembers) == 0 {
			team.deleted = true
			return true, nil
		}
		if team.LeaderId == 0 {
			team.LeaderId = team.TeamMembers[0]
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	if empty {
		return team.Ref().Stop(ctx)
	}
	return nil
}

func (r *Registry) SetRoomProjection(ctx context.Context, teamID int64, roomID string) (bool, error) {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return false, err
	}
	_, err = actor.Call(ctx, team.Ref(), func(actor.Context) (struct{}, error) {
		team.RoomId = roomID
		return struct{}{}, nil
	})
	return err == nil, err
}

func (t Team) GetPersistId() interface{} {
	return t.TeamId
}

func (t *Team) OnStop(context.Context) error {
	if t.deleted {
		_, err := mongodb.DeleteByID[Team](t.TeamId)
		return err
	}
	_, err := mongodb.Save(t)
	return err
}

func (t *Team) snapshot() Snapshot {
	return Snapshot{
		TeamID:    t.TeamId,
		LeaderID:  t.LeaderId,
		MemberIDs: append([]int64(nil), t.TeamMembers...),
		RoomID:    t.RoomId,
	}
}
