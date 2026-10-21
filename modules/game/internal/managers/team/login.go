package team

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/common/utils"
	"slices"
)

// LoginPreparation records only membership created by this cold login. Abort
// never deletes durable Team data or removes an existing member.
type LoginPreparation struct {
	team     *Team
	playerID int64
	added    bool
	created  bool
}

func (p *LoginPreparation) TeamID() int64 { return p.team.TeamId }

func (r *Registry) PrepareLogin(ctx context.Context, teamID, playerID int64, initial bool) (*LoginPreparation, error) {
	var stored *Team
	instance, err := r.lookup(ctx, teamID)
	if err != nil && !errors.Is(err, actor.ErrActorStopped) {
		return nil, err
	}
	if instance == nil {
		if !initial && teamID != 0 {
			stored, err = r.repository.FindByID(teamID)
			if err != nil {
				return nil, err
			}
		}
		if stored == nil && (!initial || teamID == 0) {
			teamID = utils.FlakeId()
		}
		instance, err = r.definition.GetOrCreate(ctx, teamID)
		if err != nil {
			return nil, err
		}
	}
	prepared := &LoginPreparation{team: instance, playerID: playerID}
	_, err = actor.CallUntilDone(ctx, instance.Ref(), func(actor.Context) (struct{}, error) {
		if instance.TeamId == 0 {
			prepared.created = stored == nil
			instance.TeamId = teamID
			if stored != nil {
				instance.LeaderId = stored.LeaderId
				instance.TeamMembers = append([]int64(nil), stored.TeamMembers...)
				instance.RoomId = stored.RoomId
			}
		}
		if (initial || prepared.created) && !slices.Contains(instance.TeamMembers, playerID) {
			instance.TeamMembers = append(instance.TeamMembers, playerID)
			prepared.added = true
			if instance.LeaderId == 0 {
				instance.LeaderId = playerID
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		return nil, errors.Join(err, prepared.Abort(context.Background()))
	}
	return prepared, nil
}

func (p *LoginPreparation) Abort(ctx context.Context) error {
	if p == nil {
		return nil
	}
	stopping := false
	_, err := actor.CallUntilDone(ctx, p.team.Ref(), func(execution actor.Context) (struct{}, error) {
		if p.added {
			p.team.TeamMembers = slices.DeleteFunc(p.team.TeamMembers, func(id int64) bool { return id == p.playerID })
			if p.team.LeaderId == p.playerID {
				p.team.LeaderId = 0
				if len(p.team.TeamMembers) > 0 {
					p.team.LeaderId = p.team.TeamMembers[0]
				}
			}
		}
		if p.created && len(p.team.TeamMembers) == 0 {
			stopping = true
			// Reject later joins in the same Actor turn that confirms emptiness.
			// Skip persistence and preserve the durable provision Intent/Team.
			return struct{}{}, p.team.Ref().ForceStop(execution)
		}
		return struct{}{}, nil
	})
	if errors.Is(err, actor.ErrActorStopped) {
		return nil
	}
	if err == nil && stopping {
		return p.team.Ref().ForceStop(ctx)
	}
	return err
}
