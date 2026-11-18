package player

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
)

// Recover 在 cleanup 解除 Session、Team 和 Room 关联后放弃隔离状态。
// 不激活替代实例；下一次登录仍走正常的加载及提交流程。
func (r *Registry) Recover(ctx context.Context, playerID int64, cleanup func(context.Context) error) error {
	if cleanup == nil {
		return fmt.Errorf("player: quarantine cleanup is nil")
	}
	unlock, err := r.recoveryMu.Lock(ctx, playerID)
	if err != nil {
		return err
	}
	defer unlock()
	target, err := r.definition.QuarantineTarget(ctx, playerID)
	if errors.Is(err, actor.ErrActorNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := cleanup(ctx); err != nil {
		return err
	}
	return r.definition.ForceStopQuarantined(ctx, target)
}
