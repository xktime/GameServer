package team

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
)

// Recover 在 cleanup 解除成员和 Room 关联后放弃隔离状态。
// 后续激活仍通过正常的创建或登录准备入口完成。
func (r *Registry) Recover(ctx context.Context, teamID int64, cleanup func(context.Context) error) error {
	if cleanup == nil {
		return fmt.Errorf("team: quarantine cleanup is nil")
	}
	unlock, err := r.recoveryMu.Lock(ctx, teamID)
	if err != nil {
		return err
	}
	defer unlock()
	target, err := r.definition.QuarantineTarget(ctx, teamID)
	if errors.Is(err, actor.ErrActorNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := r.broadcasts.stopQuarantinedTeam(ctx, teamID); err != nil {
		return err
	}
	if err := cleanup(ctx); err != nil {
		return err
	}
	return r.definition.ForceStopQuarantined(ctx, target)
}
