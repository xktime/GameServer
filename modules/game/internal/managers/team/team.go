package team

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/utils"
	"gameserver/core/log"
	"slices"
)

// Team 是 Mongo 持久化数据；运行时状态由 teamActor 持有。
type Team struct {
	TeamId      int64   `bson:"_id"`
	LeaderId    int64   `bson:"leader_id"`
	TeamMembers []int64 `bson:"team_members"`
	RoomId      string  `bson:"room_id"`
}

type teamActor struct {
	actor.BaseActor
	Team
	deleted    bool
	repository Repository
	dispatcher *broadcastActor
	broadcasts teamBroadcasts
}

var ErrTeamNotReady = errors.New("team: initialization is not complete")

type Snapshot struct {
	TeamID    int64
	LeaderID  int64
	MemberIDs []int64
	RoomID    string
}

type Registry struct {
	definition *actor.Definition[*teamActor, int64]
	broadcasts *broadcastActor
	repository Repository
	recoveryMu utils.KeyedMutex[int64]
}

func NewRegistry(scope *actor.Scope) (*Registry, error) {
	return NewRegistryWithRepository(scope, mongoTeamRepository{})
}

func NewRegistryWithRepository(scope *actor.Scope, repository Repository) (*Registry, error) {
	if repository == nil {
		return nil, fmt.Errorf("game: Team Repository is nil")
	}
	registry := &Registry{repository: repository}
	definition, err := actor.Define(scope, func(_ context.Context, teamID int64) (*teamActor, error) {
		return &teamActor{
			repository: repository,
			dispatcher: registry.broadcasts,
			broadcasts: teamBroadcasts{teamID: teamID},
		}, nil
	}, actor.DefinitionOptions{QueueCapacity: 256})
	if err != nil {
		return nil, err
	}
	registry.definition = definition
	broadcasts, err := actor.Define(scope, func(context.Context, string) (*broadcastActor, error) {
		return &broadcastActor{active: make(map[*teamBroadcasts]struct{})}, nil
	}, actor.DefinitionOptions{QueueCapacity: broadcastQueueCapacity})
	if err != nil {
		return nil, err
	}
	registry.broadcasts, err = broadcasts.GetOrCreate(context.Background(), "singleton")
	if err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *Registry) lookup(ctx context.Context, teamID int64) (*teamActor, error) {
	return r.definition.Lookup(ctx, teamID)
}

// Snapshot 只观察已有状态。离线数据直接从仓库返回，不激活 Actor。
func (r *Registry) Snapshot(ctx context.Context, teamID int64) (Snapshot, error) {
	instance, err := r.lookup(ctx, teamID)
	if errors.Is(err, actor.ErrActorNotFound) {
		stored, err := r.repository.FindByID(ctx, teamID)
		if err != nil {
			return Snapshot{}, err
		}
		if stored == nil {
			return Snapshot{}, actor.ErrActorNotFound
		}
		return stored.snapshot(), nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	return actor.Query(ctx, instance.Ref(), func(actor.Context) (Snapshot, error) {
		if err := instance.readyError(); err != nil {
			return Snapshot{}, err
		}
		return instance.snapshot(), nil
	})
}

// Stop 等待已接收的广播完成，也是持久化或删除失败后的唯一重试入口。
func (r *Registry) Stop(ctx context.Context, teamID int64) error {
	return r.definition.Stop(ctx, teamID)
}

func (r *Registry) Join(ctx context.Context, teamID int64, playerID int64) error {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return err
	}
	_, err = actor.Command(ctx, team.Ref(), func(actor.Context) (struct{}, error) {
		if err := team.readyError(); err != nil {
			return struct{}{}, err
		}
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
	var stopping actor.StopResult
	empty, err := actor.Command(ctx, team.Ref(), func(execution actor.Context) (bool, error) {
		if err := team.readyError(); err != nil {
			return false, err
		}
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
			var err error
			stopping, err = execution.StopSelf(ctx)
			return true, err
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
		return stopping.Wait(ctx)
	}
	return nil
}

func (r *Registry) SetRoomProjection(ctx context.Context, teamID int64, roomID string) error {
	team, err := r.lookup(ctx, teamID)
	if err != nil {
		return err
	}
	_, err = actor.Command(ctx, team.Ref(), func(actor.Context) (struct{}, error) {
		if err := team.readyError(); err != nil {
			return struct{}{}, err
		}
		team.RoomId = roomID
		return struct{}{}, nil
	})
	return err
}

func (t Team) GetPersistId() interface{} {
	return t.TeamId
}

func (t *teamActor) OnStop(ctx context.Context) error {
	if err := waitBroadcasts(ctx, t.dispatcher.seal(&t.broadcasts)); err != nil {
		return err
	}
	if t.deleted {
		return t.repository.DeleteByID(ctx, t.TeamId)
	}
	return t.repository.Save(ctx, &t.Team)
}

func (t *Team) snapshot() Snapshot {
	return Snapshot{
		TeamID:    t.TeamId,
		LeaderID:  t.LeaderId,
		MemberIDs: append([]int64(nil), t.TeamMembers...),
		RoomID:    t.RoomId,
	}
}

func (t *teamActor) readyError() error {
	if t.deleted {
		return actor.ErrActorStopping
	}
	if t.TeamId == 0 {
		return ErrTeamNotReady
	}
	return nil
}
