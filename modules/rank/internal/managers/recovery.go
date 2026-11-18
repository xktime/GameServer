package managers

import (
	"context"
)

// Recover 显式放弃隔离实例的未保存状态，从持久化快照恢复排行榜。
// 加载失败时保留原句柄，后续调用可继续初始化新代次。
func (h *RankManager) Recover(ctx context.Context) error {
	unlock, err := h.recoveryMu.Lock(ctx, "singleton")
	if err != nil {
		return err
	}
	defer unlock()
	instance, err := h.definition.RebuildQuarantined(ctx, "singleton", h.currentActor().Ref(), nil)
	if err != nil {
		return err
	}
	if err := instance.initializeUntilReady(ctx); err != nil {
		return err
	}
	h.mu.Lock()
	h.actor = instance
	h.mu.Unlock()
	return nil
}
