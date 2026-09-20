package player

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	gameprovision "gameserver/modules/game/internal/provision"

	"google.golang.org/protobuf/proto"
)

type Player struct {
	actor.BaseActor `bson:"-"`
	PlayerId        int64                   `bson:"_id"`
	PlayerInfo      *playermodel.PlayerInfo `bson:"player_info"`
	TeamId          int64                   `bson:"team_id"`
	TowerLevel      int32                   `bson:"tower_level"`
	agent           gate.Agent              `bson:"-"`
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
		return &Player{teams: teams, repository: repository}, nil
	})
	if err != nil {
		return nil, err
	}
	registry.definition = definition
	return registry, nil
}

// LoginProvision 根据已持久化的创建意图，
// 创建新玩家及其初始队伍。
func (r *Registry) LoginProvision(ctx context.Context, agent gate.Agent, user models.User, data gameprovision.PlayerData) error {
	return r.start(ctx, agent, user.PlayerId, func(execution actor.Context, instance *Player) error {
		instance.InitProvisionData(data)
		return instance.initTeamWithID(execution, data.TeamID)
	})
}

// Login 初始化已有玩家及其队伍。
func (r *Registry) Login(ctx context.Context, agent gate.Agent, user models.User) error {
	return r.start(ctx, agent, user.PlayerId, func(execution actor.Context, instance *Player) error {
		if err := instance.InitPlayerData(user.PlayerId); err != nil {
			return err
		}
		return instance.initTeam(execution)
	})
}

func (r *Registry) start(ctx context.Context, agent gate.Agent, playerID int64, initialize func(actor.Context, *Player) error) error {
	if agent == nil {
		return fmt.Errorf("game: Agent is nil")
	}
	if existing, err := r.definition.Lookup(ctx, playerID); err == nil {
		log.Error("玩家 Actor 已存在，停止旧代实例: %v", playerID)
		if err := existing.Ref().Stop(ctx); err != nil {
			return fmt.Errorf("stop existing player %d: %w", playerID, err)
		}
	}

	instance, err := r.definition.GetOrCreate(ctx, playerID)
	if err != nil {
		return err
	}
	_, err = actor.Call(ctx, instance.Ref(), func(execution actor.Context) (struct{}, error) {
		instance.agent = agent
		return struct{}{}, initialize(execution, instance)
	})
	if err != nil {
		if !errors.Is(err, actor.ErrOutcomeUnknown) {
			_ = instance.Ref().ForceStop(context.Background())
		}
		return err
	}
	return nil
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

func (p Player) GetPersistId() interface{} {
	return p.PlayerId
}

func (p *Player) OnStop(context.Context) error {
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

func (p *Player) initTeam(ctx context.Context) error {
	if p.TeamId != 0 {
		if _, err := p.teams.Snapshot(ctx, p.TeamId); err == nil {
			return nil
		} else if !errors.Is(err, actor.ErrActorStopped) {
			return err
		}
	}
	teamInfo, err := p.teams.Create(ctx, p.PlayerId)
	if err != nil {
		return err
	}
	p.TeamId = teamInfo.TeamID
	return p.teams.Join(ctx, teamInfo.TeamID, p.PlayerId)
}

func (p *Player) initTeamWithID(ctx context.Context, teamID int64) error {
	if teamID == 0 {
		return p.initTeam(ctx)
	}
	teamInfo, err := p.teams.GetOrCreate(ctx, teamID, p.PlayerId)
	if err != nil {
		return err
	}
	p.TeamId = teamInfo.TeamID
	return p.teams.Join(ctx, teamInfo.TeamID, p.PlayerId)
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
		p.agent.WriteMsg(msg)
		return nil
	})
}

func (p *Player) SendToClientSeq(ctx context.Context, msg proto.Message, seq uint32) error {
	return actor.Tell(ctx, p.Ref(), func(actor.Context) error {
		p.agent.WriteMsgWithSeq(msg, seq)
		return nil
	})
}

func (p *Player) Snapshot(ctx context.Context) (Snapshot, error) {
	return actor.Call(ctx, p.Ref(), func(actor.Context) (Snapshot, error) {
		var info *playermodel.PlayerInfo
		if p.PlayerInfo != nil {
			copyOfInfo := *p.PlayerInfo
			info = &copyOfInfo
		}
		return Snapshot{
			PlayerID:   p.PlayerId,
			TeamID:     p.TeamId,
			TowerLevel: p.TowerLevel,
			PlayerInfo: info,
		}, nil
	})
}
