package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
)

// Recover 先解除房间索引及 Team 投影，再放弃隔离房间的未保存状态。
// 清理失败时保留隔离代次，后续调用可重试未完成的投影。
func (m *RoomManager) Recover(ctx context.Context, roomID string) error {
	unlock, err := m.recoveryMu.Lock(ctx, roomID)
	if err != nil {
		return err
	}
	defer unlock()
	target, err := m.roomDefinition.QuarantineTarget(ctx, roomID)
	if errors.Is(err, actor.ErrActorNotFound) {
		m.markRoomStopped(roomID)
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := m.closeRoom(ctx, roomID, true); err != nil {
		return err
	}
	if err := m.roomDefinition.ForceStopQuarantined(ctx, target); err != nil {
		return err
	}
	m.markRoomStopped(roomID)
	return nil
}
