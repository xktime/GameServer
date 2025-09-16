package actor

import (
	"sync"
)

// ActorManager 统一管理所有TaskHandler实例
type ActorManager struct {
	taskHandlers map[string]*TaskHandler
	mu           sync.RWMutex
	taskTimeout  int // 任务超时时间（毫秒）
}

var (
	globalActorManager *ActorManager
)

// Init 初始化全局Actor管理器实例
func Init(milliseconds int) {
	globalActorManager = NewActorManager(milliseconds)
}

func NewActorManager(milliseconds int) *ActorManager {
	return &ActorManager{
		taskHandlers: make(map[string]*TaskHandler),
		taskTimeout:  milliseconds,
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

// GetActor 函数已移动到 task_handler.go 中，这里保留向后兼容的别名
// 建议使用 task_handler.go 中的新版本

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
	// 先获取所有TaskHandler的副本
	taskHandlers := make([]*TaskHandler, 0, len(globalActorManager.taskHandlers))
	for _, taskHandler := range globalActorManager.taskHandlers {
		taskHandlers = append(taskHandlers, taskHandler)
	}
	globalActorManager.mu.RUnlock()

	// 停止所有TaskHandler（不持有锁）
	for _, taskHandler := range taskHandlers {
		taskHandler.Stop()
	}

	// 最后清空所有TaskHandler
	globalActorManager.mu.Lock()
	globalActorManager.taskHandlers = make(map[string]*TaskHandler)
	globalActorManager.mu.Unlock()
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

// GetTaskTimeout 获取任务超时时间（毫秒）
func GetTaskTimeout() int {
	globalActorManager.mu.RLock()
	defer globalActorManager.mu.RUnlock()
	return globalActorManager.taskTimeout
}
