package managers

import (
	"context"
	"fmt"
)

// Recover 放弃隔离实例的未保存状态。cleanup 必须先清理关联的会话及领域状态；
// 清理失败时保留原隔离实例。重建失败后可使用新的上下文重试。
func (h *UserManager) Recover(ctx context.Context, cleanup func(context.Context) error) error {
	if cleanup == nil {
		return fmt.Errorf("game: quarantine cleanup is nil")
	}
	unlock, err := h.recoveryMu.Lock(ctx, "singleton")
	if err != nil {
		return err
	}
	defer unlock()
	instance, err := h.definition.RebuildQuarantined(ctx, "singleton", h.currentActor().Ref(), cleanup)
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

// RecoverPlayer 的 cleanup 负责解除该 Player 的 Session、Team 和 Room 关联。
// 成功后由下一次登录从持久化数据重新加载 Player。
func (h *UserManager) RecoverPlayer(ctx context.Context, playerID int64, cleanup func(context.Context) error) error {
	return h.players.Recover(ctx, playerID, cleanup)
}

func (m *TeamManager) Recover(ctx context.Context, teamID int64, cleanup func(context.Context) error) error {
	return m.teams.Recover(ctx, teamID, cleanup)
}
