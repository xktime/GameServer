package player

import (
	"context"
	"gameserver/common/base/actor"
)

// StoredSnapshot 只读取持久化数据，不创建或激活 Actor。
func (r *Registry) StoredSnapshot(ctx context.Context, playerID int64) (Snapshot, error) {
	stored, err := r.repository.FindByID(ctx, playerID)
	if err != nil {
		return Snapshot{}, err
	}
	if stored == nil {
		return Snapshot{}, actor.ErrActorNotFound
	}
	return stored.snapshot(), nil
}

func (r *Registry) NameExists(ctx context.Context, name string) (bool, error) {
	stored, err := r.repository.FindByName(ctx, name)
	if err != nil {
		return false, err
	}
	return stored != nil, nil
}

// StoredNames 返回持久化的非空玩家名称，不创建或激活 Actor。
func (r *Registry) StoredNames(ctx context.Context) ([]string, error) {
	players, err := r.repository.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(players))
	for _, player := range players {
		if player.PlayerInfo != nil && player.PlayerInfo.PlayerName != "" {
			names = append(names, player.PlayerInfo.PlayerName)
		}
	}
	return names, nil
}
