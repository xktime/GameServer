package managers

import (
	"context"
	"fmt"
)

// Recover 在 cleanup 关闭关联连接和会话后重建心跳管理，放弃隔离实例状态。
func (h *ConnectManager) Recover(ctx context.Context, cleanup func(context.Context) error) error {
	if cleanup == nil {
		return fmt.Errorf("login: quarantine cleanup is nil")
	}
	unlock, err := h.recoveryMu.Lock(ctx, "connect")
	if err != nil {
		return err
	}
	defer unlock()
	instance, err := h.definition.RebuildQuarantined(ctx, "connect", h.currentActor().Ref(), cleanup)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.actor = instance
	h.mu.Unlock()
	return nil
}
