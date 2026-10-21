package player

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/models"
	"gameserver/common/msg/message"
	"gameserver/modules/game/internal/managers/team"
	gameprovision "gameserver/modules/game/internal/provision"
	"gameserver/modules/game/playersession"
)

type PreparedLogin struct {
	player *Player
	team   *team.LoginPreparation
	owned  bool
}

// PrepareLogin reuses an online Player unchanged, or initializes a cold Player
// without binding a connection. UserManager serializes its complete lifetime.
func (r *Registry) PrepareLogin(ctx context.Context, user models.User, initial *gameprovision.PlayerData, online bool) (*PreparedLogin, error) {
	instance, err := r.lookup(ctx, user.PlayerId)
	if online {
		if err != nil {
			return nil, err
		}
		return &PreparedLogin{player: instance}, nil
	}
	if err == nil {
		return nil, fmt.Errorf("player %d already exists without an active session", user.PlayerId)
	}
	if !errors.Is(err, actor.ErrActorStopped) {
		return nil, err
	}
	instance, err = r.definition.GetOrCreate(ctx, user.PlayerId)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedLogin{player: instance, owned: true}
	_, err = actor.CallUntilDone(ctx, instance.Ref(), func(execution actor.Context) (struct{}, error) {
		if initial != nil {
			instance.InitProvisionData(*initial)
		} else if err := instance.InitPlayerData(user.PlayerId); err != nil {
			return struct{}{}, err
		}
		preparedTeam, err := instance.teams.PrepareLogin(execution, instance.TeamId, user.PlayerId, initial != nil)
		if err != nil {
			return struct{}{}, err
		}
		prepared.team = preparedTeam
		instance.TeamId = prepared.team.TeamID()
		return struct{}{}, nil
	})
	if err != nil {
		return nil, errors.Join(err, prepared.Abort(context.Background()))
	}
	return prepared, nil
}

func (p *PreparedLogin) Commit(ctx context.Context, attempt *playersession.Attempt, user models.User, response *message.S2C_Login) (bool, error) {
	committed := false
	_, err := actor.CallUntilDone(ctx, p.player.Ref(), func(actor.Context) (struct{}, error) {
		previous, won := attempt.Commit(user, &p.player.connection)
		committed = won
		if !committed {
			return struct{}{}, nil
		}
		p.player.preparing = false
		p.owned = false
		// Still on this Player's queue; subsequent pushes cannot overtake login.
		attempt.Reply(response)
		if previous.Agent != nil {
			previous.Agent.WriteMsg(&message.S2C_Logout{})
			previous.Agent.Close()
		}
		return struct{}{}, nil
	})
	return committed, err
}

func (p *PreparedLogin) Abort(ctx context.Context) error {
	if p == nil || !p.owned {
		return nil
	}
	// The definitive prepare/commit result is known before Abort is called.
	return errors.Join(p.player.Ref().ForceStop(ctx), p.team.Abort(ctx))
}
