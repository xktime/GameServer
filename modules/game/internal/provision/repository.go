package provision

import (
	"context"
	"errors"
	"gameserver/common/db/mongodb"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// Repository 存取持久化的玩家创建意图。
type Repository interface {
	Save(context.Context, *Intent) error
	FindByAccount(context.Context, string) (*Intent, error)
	FindPending(context.Context, time.Time) ([]Intent, error)
}

type MongoRepository struct{}

func NewMongoRepository() MongoRepository {
	return MongoRepository{}
}

func (MongoRepository) Save(ctx context.Context, intent *Intent) error {
	if intent == nil {
		return errors.New("player provision: intent is nil")
	}
	_, err := mongodb.SaveContext(ctx, intent)
	return err
}

func (MongoRepository) FindByAccount(ctx context.Context, accountID string) (*Intent, error) {
	intent, err := mongodb.FindOneContext[Intent](ctx, bson.M{"_id": accountID})
	return intent, err
}

func (MongoRepository) FindPending(ctx context.Context, now time.Time) ([]Intent, error) {
	intents, err := mongodb.FindAllContext[Intent](ctx, bson.M{"status": StatusPending})
	if err != nil {
		return nil, err
	}
	result := make([]Intent, 0, len(intents))
	for _, intent := range intents {
		if !intent.NextAttemptAt.IsZero() && intent.NextAttemptAt.After(now) {
			continue
		}
		result = append(result, intent)
	}
	return result, nil
}

// MemoryRepository 用于确定性地测试重试和幂等行为，
// 无需依赖 MongoDB。
type MemoryRepository struct {
	mu      sync.Mutex
	intents map[string]Intent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{intents: make(map[string]Intent)}
}

func (r *MemoryRepository) Save(ctx context.Context, intent *Intent) error {
	if intent == nil {
		return errors.New("player provision: intent is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.intents[intent.AccountID] = cloneIntent(*intent)
	return nil
}

func (r *MemoryRepository) FindByAccount(ctx context.Context, accountID string) (*Intent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	intent, ok := r.intents[accountID]
	if !ok {
		return nil, nil
	}
	copyOfIntent := cloneIntent(intent)
	return &copyOfIntent, nil
}

func (r *MemoryRepository) FindPending(ctx context.Context, now time.Time) ([]Intent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Intent, 0)
	for _, intent := range r.intents {
		if intent.Status == StatusPending &&
			(intent.NextAttemptAt.IsZero() || !intent.NextAttemptAt.After(now)) {
			result = append(result, cloneIntent(intent))
		}
	}
	return result, nil
}

func cloneIntent(intent Intent) Intent {
	if intent.Player.PlayerInfo != nil {
		info := *intent.Player.PlayerInfo
		intent.Player.PlayerInfo = &info
	}
	intent.Team.MemberIDs = append([]int64(nil), intent.Team.MemberIDs...)
	return intent
}
