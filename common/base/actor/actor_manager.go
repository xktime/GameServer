package actor

import (
	"sync"
)

type IActor interface {
	Start()
	Stop()
}

// ActorManager 统一管理所有Actor实例
type ActorManager struct {
	taskHandlers map[string]*TaskHandler
	mu           sync.RWMutex
}

var (
	actorFactory *ActorManager
)

// GetActorManager 获取全局Actor管理器实例（单例模式）
func Init(milliseconds int) {
	actorFactory = NewActorManager()
}

func NewActorManager() *ActorManager {
	return &ActorManager{
		taskHandlers: make(map[string]*TaskHandler),
	}
}

// RegisterActor 注册Actor到管理器
func Register(name string, actor *TaskHandler) bool {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()

	if _, exists := actorFactory.taskHandlers[name]; exists {
		return false // Actor已存在
	}

	actorFactory.taskHandlers[name] = actor
	return true
}

// UnregisterActor 从管理器注销Actor
func Unregister(name string) bool {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()

	if _, exists := actorFactory.taskHandlers[name]; exists {
		delete(actorFactory.taskHandlers, name)
		return true
	}

	return false
}

func GetActor[T IActor](actorGroup ActorGroup, uniqueID interface{}) (T, bool) {
	id := getUniqueId(actorGroup, uniqueID)
	handler, exists := GetHandler(id)
	if !exists {
		var zero T
		return zero, false
	}

	// 安全的类型断言
	if actor, ok := handler.actors[getActorNameByType[T]()].(T); ok {
		return actor, true
	}

	var zero T
	return zero, false
}

// GetActor 获取指定名称的Actor
func GetHandler(name string) (*TaskHandler, bool) {
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()

	actor, exists := actorFactory.taskHandlers[name]
	return actor, exists
}

// GetAllActors 获取所有注册的Actor
func GetAllActors() map[string]*TaskHandler {
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()

	result := make(map[string]*TaskHandler)
	for name, actor := range actorFactory.taskHandlers {
		result[name] = actor
	}
	return result
}

// StopAll 停止所有注册的Actor
func StopAll() {
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()

	for name, actor := range actorFactory.taskHandlers {
		go func(name string, actor *TaskHandler) {
			actor.Stop()
		}(name, actor)
	}
}

// GetActorCount 获取注册的Actor数量
func (am *ActorManager) GetActorCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()

	return len(am.taskHandlers)
}

// IsActorRegistered 检查Actor是否已注册
func (am *ActorManager) IsActorRegistered(name string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	_, exists := am.taskHandlers[name]
	return exists
}
