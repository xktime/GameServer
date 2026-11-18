package player

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"

	"google.golang.org/protobuf/proto"
)

// Player 是 Mongo 持久化数据；类型名决定现有集合名。
type Player struct {
	PlayerId   int64                   `bson:"_id"`
	PlayerInfo *playermodel.PlayerInfo `bson:"player_info"`
	TeamId     int64                   `bson:"team_id"`
	TowerLevel int32                   `bson:"tower_level"`
}

type playerActor struct {
	actor.BaseActor
	Player
	connection playersession.Slot
	preparing  bool
	teams      *team.Registry
	repository Repository
}

var ErrPlayerNotReady = errors.New("player: login is not committed")

type Snapshot struct {
	PlayerID   int64
	TeamID     int64
	TowerLevel int32
	PlayerInfo *playermodel.PlayerInfo
}

type Registry struct {
	definition *actor.Definition[*playerActor, int64]
	repository Repository
	recoveryMu utils.KeyedMutex[int64]
}

func NewRegistry(scope *actor.Scope, teams *team.Registry) (*Registry, error) {
	return NewRegistryWithRepository(scope, teams, mongoPlayerRepository{})
}

func NewRegistryWithRepository(scope *actor.Scope, teams *team.Registry, repository Repository) (*Registry, error) {
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	if repository == nil {
		return nil, fmt.Errorf("game: Player Repository is nil")
	}
	registry := &Registry{repository: repository}
	definition, err := actor.Define(scope, func(context.Context, int64) (*playerActor, error) {
		return &playerActor{teams: teams, repository: repository, preparing: true}, nil
	}, actor.DefinitionOptions{QueueCapacity: 256})
	if err != nil {
		return nil, err
	}
	registry.definition = definition
	return registry, nil
}

func (r *Registry) lookup(ctx context.Context, playerID int64) (*playerActor, error) {
	return r.definition.Lookup(ctx, playerID)
}

func (r *Registry) Snapshot(ctx context.Context, playerID int64) (Snapshot, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return Snapshot{}, err
	}
	return actor.Query(ctx, instance.Ref(), func(actor.Context) (Snapshot, error) {
		if instance.preparing {
			return Snapshot{}, ErrPlayerNotReady
		}
		return instance.snapshot(), nil
	})
}

func (r *Registry) ModifyName(ctx context.Context, playerID int64, name string) (message.Result, string, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return message.Result_Fail, "", err
	}
	type result struct {
		status message.Result
		name   string
	}
	response, err := actor.Command(ctx, instance.Ref(), func(actor.Context) (result, error) {
		if instance.preparing {
			return result{}, ErrPlayerNotReady
		}
		if len(name) < 2 || len(name) > 20 {
			return result{status: message.Result_Illegal, name: instance.PlayerInfo.PlayerName}, nil
		}
		instance.PlayerInfo.PlayerName = name
		return result{status: message.Result_Success, name: name}, nil
	})
	return response.status, response.name, err
}

func (r *Registry) ModifyAvatarSuffix(ctx context.Context, playerID int64, avatar string) (message.Result, string, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return message.Result_Fail, "", err
	}
	type result struct {
		status message.Result
		avatar string
	}
	response, err := actor.Command(ctx, instance.Ref(), func(actor.Context) (result, error) {
		if instance.preparing {
			return result{}, ErrPlayerNotReady
		}
		instance.PlayerInfo.AvatarSuffix = "." + avatar
		return result{status: message.Result_Success, avatar: instance.PlayerInfo.AvatarSuffix}, nil
	})
	return response.status, response.avatar, err
}

func (r *Registry) SendToClient(ctx context.Context, playerID int64, msg proto.Message) error {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return err
	}
	return actor.Tell(ctx, instance.Ref(), func(actor.Context) error {
		if instance.preparing {
			return ErrPlayerNotReady
		}
		if agent := instance.connection.Agent(); agent != nil {
			agent.WriteMsg(msg)
		}
		return nil
	})
}

func (r *Registry) Stop(ctx context.Context, playerID int64) error {
	return r.definition.Stop(ctx, playerID)
}

// Offline 在同一任务内取得最终快照并关闭入口，再等待本次保存结果。
// 保存失败时仍返回快照供 Team 清理；原实例留在运行时，等待显式 Stop 重试。
func (r *Registry) Offline(ctx context.Context, playerID int64) (Snapshot, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return Snapshot{}, err
	}
	var stopping actor.StopResult
	snapshot, err := actor.Command(ctx, instance.Ref(), func(execution actor.Context) (Snapshot, error) {
		if instance.preparing {
			return Snapshot{}, ErrPlayerNotReady
		}
		if agent := instance.connection.Agent(); agent != nil {
			agent.WriteMsg(&message.S2C_Logout{})
		}
		var err error
		stopping, err = execution.StopSelf(ctx)
		return instance.snapshot(), err
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, stopping.Wait(ctx)
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

func (p *playerActor) OnStop(ctx context.Context) error {
	if p.preparing {
		return nil
	}
	return p.repository.Save(ctx, &p.Player)
}

func (p *playerActor) InitPlayerData(ctx context.Context, playerID int64) error {
	existing, err := p.repository.FindByID(ctx, playerID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("老玩家数据不存在: %v", playerID)
	}
	p.PlayerId = existing.PlayerId
	p.PlayerInfo = existing.PlayerInfo
	p.TeamId = existing.TeamId
	p.TowerLevel = existing.TowerLevel
	return nil
}

func (p *playerActor) InitProvisionData(data gameprovision.PlayerData) {
	p.PlayerId = data.PlayerID
	p.TeamId = data.TeamID
	p.TowerLevel = data.TowerLevel
	p.PlayerInfo = nil
	if data.PlayerInfo != nil {
		copyOfInfo := *data.PlayerInfo
		p.PlayerInfo = &copyOfInfo
	}
}

func (p Player) snapshot() Snapshot {
	var info *playermodel.PlayerInfo
	if p.PlayerInfo != nil {
		copyOfInfo := *p.PlayerInfo
		info = &copyOfInfo
	}
	return Snapshot{PlayerID: p.PlayerId, TeamID: p.TeamId, TowerLevel: p.TowerLevel, PlayerInfo: info}
}
