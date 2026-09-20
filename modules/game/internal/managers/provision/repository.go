package provision

import (
	"errors"
	"gameserver/common/db/mongodb"
	model "gameserver/modules/game/internal/provision"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// Repository 存取持久化的玩家创建意图。
type Repository interface {
	Save(*model.Intent) error
	FindByAccount(string) (*model.Intent, error)
	FindPending(time.Time) ([]model.Intent, error)
}

type MongoRepository struct{}

func NewMongoRepository() MongoRepository {
	return MongoRepository{}
}

func (MongoRepository) Save(intent *model.Intent) error {
	if intent == nil {
		return errors.New("player provision: intent is nil")
	}
	_, err := mongodb.Save(intent)
	return err
}

func (MongoRepository) FindByAccount(accountID string) (*model.Intent, error) {
	intent, err := mongodb.FindOne[model.Intent](bson.M{"_id": accountID})
	return intent, err
}

func (MongoRepository) FindPending(now time.Time) ([]model.Intent, error) {
	intents, err := mongodb.FindAll[model.Intent](bson.M{"status": model.StatusPending})
	if err != nil {
		return nil, err
	}
	result := make([]model.Intent, 0, len(intents))
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
	intents map[string]model.Intent
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{intents: make(map[string]model.Intent)}
}

func (r *MemoryRepository) Save(intent *model.Intent) error {
	if intent == nil {
		return errors.New("player provision: intent is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.intents[intent.AccountID] = cloneIntent(*intent)
	return nil
}

func (r *MemoryRepository) FindByAccount(accountID string) (*model.Intent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	intent, ok := r.intents[accountID]
	if !ok {
		return nil, nil
	}
	copyOfIntent := cloneIntent(intent)
	return &copyOfIntent, nil
}

func (r *MemoryRepository) FindPending(now time.Time) ([]model.Intent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]model.Intent, 0)
	for _, intent := range r.intents {
		if intent.Status == model.StatusPending &&
			(intent.NextAttemptAt.IsZero() || !intent.NextAttemptAt.After(now)) {
			result = append(result, cloneIntent(intent))
		}
	}
	return result, nil
}

func cloneIntent(intent model.Intent) model.Intent {
	if intent.Player.PlayerInfo != nil {
		info := *intent.Player.PlayerInfo
		intent.Player.PlayerInfo = &info
	}
	intent.Team.MemberIDs = append([]int64(nil), intent.Team.MemberIDs...)
	return intent
}
