package actor

import (
	"reflect"
	"sync"
)

// todo 启动时自动调用Init
// todo ontimer实现
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

func GetActor[T any](actorGroup ActorGroup, uniqueID interface{}) (*T, bool) {
	id := getUniqueId(actorGroup, uniqueID)
	handler, exists := GetHandler(id)
	if !exists {
		return nil, false
	}

	// 安全的类型断言
	name := getActorNameByType[T]()
	if actor, ok := handler.actors[name]; ok {
		// 使用反射进行类型转换
		actorValue := reflect.ValueOf(actor)
		var zero T
		zeroType := reflect.TypeOf(zero)

		// 如果存储的是指针类型
		if actorValue.Kind() == reflect.Ptr {
			// 检查指针指向的类型是否匹配
			if actorValue.Type().Elem() == zeroType {
				// 使用反射进行安全的类型转换
				result := reflect.New(zeroType)
				result.Elem().Set(actorValue.Elem())
				return result.Interface().(*T), true
			}
		}

		// 如果存储的是值类型
		if actorValue.Type() == zeroType {
			// 创建指针并返回
			ptr := reflect.New(zeroType)
			ptr.Elem().Set(actorValue)
			return ptr.Interface().(*T), true
		}
	}

	return nil, false
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
