package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
)

// Recover 在 cleanup 处理未完成 Match Attempt 的 Room/Team 关联后重建匹配队列。
func (h *MatchManager) Recover(ctx context.Context, cleanup func(context.Context) error) error {
	if cleanup == nil {
		return fmt.Errorf("match: quarantine cleanup is nil")
	}
	unlock, err := h.recoveryMu.Lock(ctx, "singleton")
	if err != nil {
		return err
	}
	defer unlock()
	previous := h.currentActor()
	instance, err := h.definition.RebuildQuarantined(ctx, "singleton", previous.Ref(), cleanup)
	if err != nil {
		return err
	}
	// 旧代次已停止，计数器不再变化；发布前恢复，避免与仍在运行的 Room 重复。
	_, err = actor.Command(ctx, instance.Ref(), func(actor.Context) (struct{}, error) {
		instance.nextSyntheticID = previous.nextSyntheticID
		return struct{}{}, nil
	})
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.actor = instance
	h.mu.Unlock()
	return nil
}
