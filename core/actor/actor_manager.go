package actor_manager

import (
	"fmt"
	"gameserver/core/log"
	"reflect"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
)

// ActorFactory 用于创建和管理 actor
// 支持分组、标签、批量操作
type ActorManager struct {
	mu       sync.RWMutex
	actors   map[string]interface{}             // name -> meta
	groups   map[ActorGroup]map[string]struct{} // group -> set of names
	groupPID map[ActorGroup]*actor.PID          // group -> group actor PID
}

// NewActorFactory 创建一个新的 ActorFactory
func NewActorManager() *ActorManager {
	return &ActorManager{
		actors:   make(map[string]interface{}),
		groups:   make(map[ActorGroup]map[string]struct{}),
		groupPID: make(map[ActorGroup]*actor.PID),
	}
}

var (
	actorFactory *ActorManager
	system       *actor.ActorSystem
	context      *actor.RootContext
	timeout      time.Duration
)

func Init(milliseconds int) {
	actorFactory = NewActorManager()
	system = actor.NewActorSystem()
	context = system.Root
	timeout = time.Duration(milliseconds) * time.Millisecond
}

// Register 注册并启动一个 actor
func Register[T any](uniqueID interface{}, group ActorGroup, initFunc ...func(*T)) (*ActorMeta[T], error) {
	name := getUniqueId[T](uniqueID)
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	if _, exists := actorFactory.actors[name]; exists {
		var groupInfo string
		for group, names := range actorFactory.groups {
			if _, ok := names[name]; ok {
				groupInfo = fmt.Sprintf("%v", group)
				break
			}
		}
		log.Release("Actor 已存在: %v, 所属Group: %v", name, groupInfo)
		return nil, nil
	}

	// 1. group actor 不存在则创建
	// 为每个uniqueId创建独立的group，确保同一个uniqueId下的actor在各自的group下执行
	groupKey := getUniqueGroup(group, uniqueID)
	var groupPID *actor.PID
	if existingPID, ok := actorFactory.groupPID[groupKey]; ok {
		groupPID = existingPID
	} else {
		props := actor.PropsFromProducer(func() actor.Actor {
			return &GroupActor{}
		})
		groupPID = context.Spawn(props)
		actorFactory.groupPID[groupKey] = groupPID
	}

	// 2. 创建子actor - 自动处理值类型和指针类型
	instance := any(new(T))
	actorImpl := instance.(actor.Actor)
	props := actor.PropsFromProducer(func() actor.Actor {
		return actorImpl
	})

	pid := context.Spawn(props)

	// 3. 注册子actor到group actor
	if groupPID != nil {
		context.Send(groupPID, &RegisterChild{ID: name, PID: pid})
	}

	meta := &ActorMeta[T]{
		ID:    name,
		PID:   pid,
		Group: groupKey,
		Actor: instance.(*T),
		Tags:  make(map[string]struct{}),
	}

	// 数据初始化
	if len(initFunc) > 0 {
		for _, f := range initFunc {
			f(meta.Actor)
		}
	}
	actorFactory.actors[name] = meta
	if groupKey != "" {
		if _, ok := actorFactory.groups[groupKey]; !ok {
			actorFactory.groups[groupKey] = make(map[string]struct{})
		}
		actorFactory.groups[groupKey][name] = struct{}{}
	}
	return meta, nil
}

func GetGroupPID(group ActorGroup, uniqueID interface{}) *actor.PID {
	// 拼接完整的group key
	groupKey := getUniqueGroup(group, uniqueID)
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()

	// 直接查找完整的group key
	return actorFactory.groupPID[groupKey]
}

func Get[T any](uniqueID interface{}) *T {
	meta := GetMeta[T](uniqueID)
	if meta == nil {
		return nil
	}
	return meta.Actor
}

func GetMeta[T any](uniqueID interface{}) *ActorMeta[T] {
	id := getUniqueId[T](uniqueID)
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()
	if meta, ok := actorFactory.actors[id]; ok {
		return meta.(*ActorMeta[T])
	}
	return nil
}

func RequestFuture[T any](uniqueID interface{}, methodName string, args []interface{}) *actor.Future {
	meta := GetMeta[T](uniqueID)
	if meta == nil {
		return nil
	}

	// 通过group actor队列执行
	actorFactory.mu.RLock()
	groupPID := actorFactory.groupPID[meta.Group]
	actorFactory.mu.RUnlock()
	if groupPID == nil {
		return nil
	}
	if !meta.checkMethod(methodName) {
		log.Error("Send: 传入的method: %v, 不是 %v 的方法", methodName, reflect.TypeOf(meta.Actor))
		return nil
	}
	if args == nil {
		args = make([]interface{}, 0)
	}
	args = append([]interface{}{meta.Actor}, args...)
	// 创建Future
	future := context.RequestFuture(groupPID, &QueuedMsg{
		ID:              meta.ID,
		MethodName:      methodName,
		Params:          args,
		IsRequestFuture: true,
	}, timeout)

	return future
}

// Send 发送消息，group下的actor通过group actor串行调度
func Send[T any](uniqueID interface{}, methodName string, args []interface{}) {
	meta := GetMeta[T](uniqueID)
	if meta == nil {
		log.Error("Send: Actor with ID %s not found", uniqueID)
		return
	}
	meta.Send(methodName, args)
}

// Stop 停止并移除指定 actor
func Stop[T any](uniqueID interface{}) {
	id := getUniqueId[T](uniqueID)
	meta := GetMeta[T](uniqueID)
	if meta == nil {
		return
	}
	saveMeta(meta)
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	context.Stop(meta.PID)
	delete(actorFactory.actors, id)
	if meta.Group != "" {
		delete(actorFactory.groups[meta.Group], id)
		if len(actorFactory.groups[meta.Group]) == 0 {
			// group下无子actor时，停止group actor
			if groupPID, ok := actorFactory.groupPID[meta.Group]; ok {
				context.Stop(groupPID)
				delete(actorFactory.groupPID, meta.Group)
			}
			delete(actorFactory.groups, meta.Group)
		}
	}
}

// StopGroup 停止并移除某个分组下的所有 actor
// 停止指定group下的所有actor并移除group
func StopGroup(group ActorGroup, uniqueID interface{}) {
	groupKey := getUniqueGroup(group, uniqueID)
	log.Debug("Stop group actor %s", groupKey)
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()

	// 直接获取group下的所有actor名称
	actorNames, exists := actorFactory.groups[groupKey]
	if !exists {
		return // 该group不存在
	}

	// 停止该group下的所有actor
	for id := range actorNames {
		meta, ok := actorFactory.actors[id]
		if !ok {
			continue
		}
		saveMeta(meta)
		metaValue := reflect.ValueOf(meta)
		if metaValue.Kind() == reflect.Ptr && !metaValue.IsNil() {
			pidField := metaValue.Elem().FieldByName("PID")
			if pidField.IsValid() {
				pid := pidField.Interface().(*actor.PID)
				context.Stop(pid)
			}

			// 从actors映射中删除
			log.Debug("Stop actor %s", id)
			delete(actorFactory.actors, id)
		}
	}

	// 停止group actor
	if groupPID, ok := actorFactory.groupPID[groupKey]; ok {
		context.Stop(groupPID)
		delete(actorFactory.groupPID, groupKey)
	}

	// 从groups映射中删除
	delete(actorFactory.groups, groupKey)
}

// StopAll 停止并移除所有 actor
func StopAll() {
	SaveAllActorData()
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	for _, meta := range actorFactory.actors {
		metaValue := reflect.ValueOf(meta)
		if metaValue.Kind() == reflect.Ptr && !metaValue.IsNil() {
			pidField := metaValue.Elem().FieldByName("PID")
			if pidField.IsValid() {
				pid := pidField.Interface().(*actor.PID)
				context.Stop(pid)
			}
		}
	}
	for _, groupPID := range actorFactory.groupPID {
		context.Stop(groupPID)
	}
	actorFactory.actors = make(map[string]interface{})
	actorFactory.groups = make(map[ActorGroup]map[string]struct{})
	actorFactory.groupPID = make(map[ActorGroup]*actor.PID)
}

func getUniqueId[T any](uniqueID interface{}) string {
	return fmt.Sprintf("%s_%v", getId[T](), uniqueID)
}

func getUniqueGroup(group ActorGroup, uniqueID interface{}) ActorGroup {
	return ActorGroup(fmt.Sprintf("%s_%v", group, uniqueID))
}

func getId[T any]() string {
	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}
