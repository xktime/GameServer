package managers

import (
	"context"
	"errors"
	"gameserver/common/base/actor"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	"testing"
	"time"
)

func TestUserManagerInitializationCommandCanRetry(t *testing.T) {
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("user-manager-initialization-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	teams, err := team.NewRegistry(scope)
	if err != nil {
		t.Fatalf("创建 Team Registry: %v", err)
	}
	players, err := player.NewRegistry(scope, teams)
	if err != nil {
		t.Fatalf("创建 Player Registry: %v", err)
	}

	loadFailure := errors.New("database unavailable")
	attempts := 0
	manager, err := newUserManager(context.Background(), scope, players, teams, func() ([]player.Player, error) {
		attempts++
		if attempts == 1 {
			return nil, loadFailure
		}
		return []player.Player{{PlayerInfo: &playermodel.PlayerInfo{PlayerName: "重试成功"}}}, nil
	})
	if err != nil {
		t.Fatalf("初始化 UserManager: %v", err)
	}

	if !manager.initialized || !manager.nameCache["重试成功"] || attempts != 2 {
		t.Fatalf("重试结果不正确: initialized=%t names=%v attempts=%d", manager.initialized, manager.nameCache, attempts)
	}
}
