package actor

import (
	"sync"
)

// todo 启动时自动调用Init
type IActor interface {
	Init()
	Stop()
}

// ActorManager 统一管理所有TaskHandler实例
type ActorManager struct {
	taskHandlers map[string]*TaskHandler
	mu           sync.RWMutex
}

var (
	globalActorManager *ActorManager
)

// Init 初始化全局Actor管理器实例
func Init(milliseconds int) {
	globalActorManager = NewActorManager()
}

func NewActorManager() *ActorManager {
	return &ActorManager{
		taskHandlers: make(map[string]*TaskHandler),
	}
}

// Register 注册TaskHandler到管理器
func Register(name string, taskHandler *TaskHandler) bool {
	globalActorManager.mu.Lock()
	defer globalActorManager.mu.Unlock()

	if _, exists := globalActorManager.taskHandlers[name]; exists {
		return false // TaskHandler已存在
	}

	globalActorManager.taskHandlers[name] = taskHandler
	return true
}

// Unregister 从管理器注销TaskHandler
func Unregister(name string) bool {
	globalActorManager.mu.Lock()
	defer globalActorManager.mu.Unlock()

	if _, exists := globalActorManager.taskHandlers[name]; exists {
		delete(globalActorManager.taskHandlers, name)
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

// GetHandler 获取指定名称的TaskHandler
func GetHandler(name string) (*TaskHandler, bool) {
	globalActorManager.mu.RLock()
	defer globalActorManager.mu.RUnlock()

	taskHandler, exists := globalActorManager.taskHandlers[name]
	return taskHandler, exists
}

// GetAllTaskHandlers 获取所有注册的TaskHandler
func GetAllTaskHandlers() map[string]*TaskHandler {
	globalActorManager.mu.RLock()
	defer globalActorManager.mu.RUnlock()

	result := make(map[string]*TaskHandler)
	for name, taskHandler := range globalActorManager.taskHandlers {
		result[name] = taskHandler
	}
	return result
}

// StopAll 停止所有注册的TaskHandler
func StopAll() {
	globalActorManager.mu.RLock()
	defer globalActorManager.mu.RUnlock()

	for name, taskHandler := range globalActorManager.taskHandlers {
		go func(name string, taskHandler *TaskHandler) {
			taskHandler.Stop()
		}(name, taskHandler)
	}
}

// GetTaskHandlerCount 获取注册的TaskHandler数量
func (am *ActorManager) GetTaskHandlerCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()

	return len(am.taskHandlers)
}

// IsTaskHandlerRegistered 检查TaskHandler是否已注册
func (am *ActorManager) IsTaskHandlerRegistered(name string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	_, exists := am.taskHandlers[name]
	return exists
}
