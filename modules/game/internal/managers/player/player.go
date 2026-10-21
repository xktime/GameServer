package player

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"

	"google.golang.org/protobuf/proto"
)

type Player struct {
	actor.BaseActor `bson:"-"`
	PlayerId        int64                   `bson:"_id"`
	PlayerInfo      *playermodel.PlayerInfo `bson:"player_info"`
	TeamId          int64                   `bson:"team_id"`
	TowerLevel      int32                   `bson:"tower_level"`
	connection      playersession.Slot      `bson:"-"`
	preparing       bool                    `bson:"-"`
	teams           *team.Registry          `bson:"-"`
	repository      playerRepository        `bson:"-"`
}

type Snapshot struct {
	PlayerID   int64
	TeamID     int64
	TowerLevel int32
	PlayerInfo *playermodel.PlayerInfo
}

type Registry struct {
	definition *actor.Definition[*Player, int64]
}

func NewRegistry(scope *actor.Scope, teams *team.Registry) (*Registry, error) {
	return newRegistry(scope, teams, mongoPlayerRepository{})
}

func newRegistry(scope *actor.Scope, teams *team.Registry, repository playerRepository) (*Registry, error) {
	if teams == nil {
		return nil, fmt.Errorf("game: Team Registry is nil")
	}
	if repository == nil {
		return nil, fmt.Errorf("game: Player Repository is nil")
	}
	registry := &Registry{}
	definition, err := actor.Define(scope, actor.Player, func(context.Context, int64) (*Player, error) {
		return &Player{teams: teams, repository: repository, preparing: true}, nil
	})
	if err != nil {
		return nil, err
	}
	registry.definition = definition
	return registry, nil
}

func (r *Registry) lookup(ctx context.Context, playerID int64) (*Player, error) {
	return r.definition.Lookup(ctx, playerID)
}

func (r *Registry) Snapshot(ctx context.Context, playerID int64) (Snapshot, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return Snapshot{}, err
	}
	return instance.Snapshot(ctx)
}

func (r *Registry) ModifyName(ctx context.Context, playerID int64, name string) (message.Result, string, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return message.Result_Fail, "", err
	}
	return instance.ModifyName(ctx, name)
}

func (r *Registry) ModifyAvatarSuffix(ctx context.Context, playerID int64, avatar string) (message.Result, string, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return message.Result_Fail, "", err
	}
	return instance.ModifyAvatarSuffix(ctx, avatar)
}

func (r *Registry) SendToClient(ctx context.Context, playerID int64, msg proto.Message) error {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return err
	}
	return instance.SendToClient(ctx, msg)
}

func (r *Registry) SendToClientSeq(ctx context.Context, playerID int64, msg proto.Message, seq uint32) error {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return err
	}
	return instance.SendToClientSeq(ctx, msg, seq)
}

func (r *Registry) Stop(ctx context.Context, playerID int64) error {
	instance, err := r.lookup(ctx, playerID)
	if errors.Is(err, actor.ErrActorStopped) {
		return nil
	}
	if err != nil {
		return err
	}
	return instance.Ref().Stop(ctx)
}

// Offline waits for the Player queue before taking the final snapshot and
// stopping it. UserManager must not start a cold replacement on an unknown
// outcome from a timed-out snapshot operation.
// A successful snapshot is returned even if stopping fails, so Team cleanup
// can proceed independently of Player persistence.
func (r *Registry) Offline(ctx context.Context, playerID int64) (Snapshot, error) {
	instance, err := r.lookup(ctx, playerID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := actor.CallUntilDone(ctx, instance.Ref(), func(actor.Context) (Snapshot, error) {
		if agent := instance.connection.Agent(); agent != nil {
			agent.WriteMsg(&message.S2C_Logout{})
		}
		return instance.snapshot(), nil
	})
	return snapshot, errors.Join(err, instance.Ref().Stop(ctx))
}

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

func (p *Player) OnStop(context.Context) error {
	if p.preparing {
		return nil
	}
	return p.repository.Save(p)
}

func (p *Player) InitPlayerData(playerID int64) error {
	existing, err := p.repository.FindByID(playerID)
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

func (p *Player) InitProvisionData(data gameprovision.PlayerData) {
	p.PlayerId = data.PlayerID
	p.TeamId = data.TeamID
	p.TowerLevel = data.TowerLevel
	p.PlayerInfo = nil
	if data.PlayerInfo != nil {
		copyOfInfo := *data.PlayerInfo
		p.PlayerInfo = &copyOfInfo
	}
}

func (p *Player) ModifyName(ctx context.Context, name string) (message.Result, string, error) {
	type result struct {
		status message.Result
		name   string
	}
	response, err := actor.Call(ctx, p.Ref(), func(actor.Context) (result, error) {
		if len(name) < 2 || len(name) > 20 {
			return result{status: message.Result_Illegal, name: p.PlayerInfo.PlayerName}, nil
		}
		p.PlayerInfo.PlayerName = name
		return result{status: message.Result_Success, name: name}, nil
	})
	return response.status, response.name, err
}

func (p *Player) ModifyAvatarSuffix(ctx context.Context, avatar string) (message.Result, string, error) {
	type result struct {
		status message.Result
		avatar string
	}
	response, err := actor.Call(ctx, p.Ref(), func(actor.Context) (result, error) {
		p.PlayerInfo.AvatarSuffix = "." + avatar
		return result{status: message.Result_Success, avatar: p.PlayerInfo.AvatarSuffix}, nil
	})
	return response.status, response.avatar, err
}

func (p *Player) SendToClient(ctx context.Context, msg proto.Message) error {
	return actor.Tell(ctx, p.Ref(), func(actor.Context) error {
		if agent := p.connection.Agent(); agent != nil {
			agent.WriteMsg(msg)
		}
		return nil
	})
}

func (p *Player) SendToClientSeq(ctx context.Context, msg proto.Message, seq uint32) error {
	return actor.Tell(ctx, p.Ref(), func(actor.Context) error {
		if agent := p.connection.Agent(); agent != nil {
			agent.WriteMsgWithSeq(msg, seq)
		}
		return nil
	})
}

func (p *Player) Snapshot(ctx context.Context) (Snapshot, error) {
	return actor.Call(ctx, p.Ref(), func(actor.Context) (Snapshot, error) {
		return p.snapshot(), nil
	})
}

func (p *Player) snapshot() Snapshot {
	var info *playermodel.PlayerInfo
	if p.PlayerInfo != nil {
		copyOfInfo := *p.PlayerInfo
		info = &copyOfInfo
	}
	return Snapshot{PlayerID: p.PlayerId, TeamID: p.TeamId, TowerLevel: p.TowerLevel, PlayerInfo: info}
}
