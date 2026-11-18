package team

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/core/log"
	"sync"
)

const broadcastQueueCapacity = 256

// broadcastActor 在独立有界邮箱内派发消息，不占用 Team 的状态邮箱。
// mu 仅保护各代 Team 的接收与完成计数，不在锁内等待或发送。
type broadcastActor struct {
	actor.BaseActor
	mu     sync.Mutex
	active map[*teamBroadcasts]struct{}
}

// 每个 Team 实例独占一份，字段由派发器的 mu 保护。
type teamBroadcasts struct {
	teamID  int64
	sealed  bool
	pending int
	drained chan struct{}
}

func (b *broadcastActor) begin(team *teamBroadcasts) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if team.sealed {
		return actor.ErrActorStopping
	}
	if team.pending == 0 {
		team.drained = make(chan struct{})
		b.active[team] = struct{}{}
	}
	team.pending++
	return nil
}

func (b *broadcastActor) complete(team *teamBroadcasts) {
	b.mu.Lock()
	defer b.mu.Unlock()
	team.pending--
	if team.pending == 0 {
		delete(b.active, team)
		close(team.drained)
	}
}

func (b *broadcastActor) seal(team *teamBroadcasts) <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	team.sealed = true
	return team.drained
}

func waitBroadcasts(ctx context.Context, drained <-chan struct{}) error {
	if drained == nil {
		return nil
	}
	select {
	case <-drained:
		return nil
	default:
	}
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// 隔离实例不能再接收 Team 任务，只需封闭并排空其仍在途的广播。
func (b *broadcastActor) stopQuarantinedTeam(ctx context.Context, teamID int64) error {
	b.mu.Lock()
	var pending []<-chan struct{}
	for team := range b.active {
		if team.teamID == teamID {
			team.sealed = true
			pending = append(pending, team.drained)
		}
	}
	b.mu.Unlock()
	for _, drained := range pending {
		if err := waitBroadcasts(ctx, drained); err != nil {
			return err
		}
	}
	return nil
}

// StopBroadcasts 关闭派发入口并排空已接收任务，必须先于 Player Scope 停止。
func (r *Registry) StopBroadcasts(ctx context.Context) error {
	return r.broadcasts.Ref().Stop(ctx)
}

// Broadcast 先在已有 Team 的邮箱内复制成员，再提交到派发邮箱。
// 返回成功只表示派发已接收；随后取消调用方不撤销广播。
func (r *Registry) Broadcast(ctx context.Context, teamID int64, send func(context.Context, int64) error) error {
	if send == nil {
		return fmt.Errorf("team: broadcast sender is nil")
	}
	instance, err := r.lookup(ctx, teamID)
	if err != nil {
		return err
	}
	// 预留是生命周期状态修改，接收后必须等到完成，不能因查询超时漏掉计数。
	members, err := actor.Command(ctx, instance.Ref(), func(actor.Context) ([]int64, error) {
		if err := instance.readyError(); err != nil {
			return nil, err
		}
		if err := r.broadcasts.begin(&instance.broadcasts); err != nil {
			return nil, err
		}
		return append([]int64(nil), instance.TeamMembers...), nil
	})
	if err != nil {
		return err
	}
	err = actor.Tell(ctx, r.broadcasts.Ref(), func(execution actor.Context) error {
		defer r.broadcasts.complete(&instance.broadcasts)
		for _, memberID := range members {
			if err := sendBroadcastMember(execution, memberID, send); err != nil && !errors.Is(err, actor.ErrActorNotFound) {
				log.Error("发送队伍 %d 消息给玩家 %d 失败: %v", teamID, memberID, err)
			}
		}
		return nil
	})
	if err != nil {
		r.broadcasts.complete(&instance.broadcasts)
	}
	return err
}

// 派发器没有领域状态，单个回调的 panic 不应隔离所有 Team 的广播。
func sendBroadcastMember(ctx context.Context, memberID int64, send func(context.Context, int64) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", actor.ErrHandlerPanic, recovered)
		}
	}()
	return send(ctx, memberID)
}
