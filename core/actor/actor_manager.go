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
	actors   map[string]*ActorMeta              // name -> meta
	groups   map[ActorGroup]map[string]struct{} // group -> set of names
	groupPID map[ActorGroup]*actor.PID          // group -> group actor PID
}

// NewActorFactory 创建一个新的 ActorFactory
func NewActorManager() *ActorManager {
	return &ActorManager{
		actors:   make(map[string]*ActorMeta),
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

type InitFunc func(actor.Actor)

// Register 注册并启动一个 actor
func Register[T any](uniqueID string, group ActorGroup, initFunc ...InitFunc) (*ActorMeta, error) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	name := getUniqueName[T](uniqueID)
	if _, exists := actorFactory.actors[name]; exists {
		return nil, nil // 已存在
	}

	// 1. group actor 不存在则创建
	var groupPID *actor.PID
	if existingPID, ok := actorFactory.groupPID[group]; ok {
		groupPID = existingPID
	} else {
		props := actor.PropsFromProducer(func() actor.Actor {
			return &GroupActor{}
		})
		groupPID = context.Spawn(props)
		actorFactory.groupPID[group] = groupPID
	}

	// 2. 创建子actor - 自动处理值类型和指针类型
	actorImpl := any(new(T)).(actor.Actor)
	if len(initFunc) > 0 {
		initFunc[0](actorImpl)
	}
	props := actor.PropsFromProducer(func() actor.Actor {
		return actorImpl
	})
	pid := context.Spawn(props)

	// 3. 注册子actor到group actor
	if groupPID != nil {
		context.Send(groupPID, &RegisterChild{ID: name, PID: pid})
	}

	meta := &ActorMeta{
		ID:    name,
		PID:   pid,
		Group: group,
		Actor: actorImpl,
		Tags:  make(map[string]struct{}),
	}
	actorFactory.actors[name] = meta
	if group != "" {
		if _, ok := actorFactory.groups[group]; !ok {
			actorFactory.groups[group] = make(map[string]struct{})
		}
		actorFactory.groups[group][name] = struct{}{}
	}
	return meta, nil
}

func GetGroupPID(group ActorGroup) *actor.PID {
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()
	return actorFactory.groupPID[group]
}

// Get 获取 actor 的 PID
func Get[T any](uniqueID string) *actor.PID {
	actorFactory.mu.RLock()
	defer actorFactory.mu.RUnlock()
	id := getUniqueName[T](uniqueID)
	if meta, ok := actorFactory.actors[id]; ok {
		return meta.PID
	}
	return nil
}

func RequestFuture[T any](uniqueID string, method interface{}, args []interface{}) *actor.Future {
	actorFactory.mu.RLock()
	id := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[id]
	actorFactory.mu.RUnlock()
	if !ok {
		return nil
	}

	// 通过group actor队列执行
	actorFactory.mu.RLock()
	groupPID := actorFactory.groupPID[meta.Group]
	actorFactory.mu.RUnlock()
	if groupPID == nil {
		return nil
	}
	if !meta.checkMethod(method) {
		log.Error("Send: 传入的method: %v, 不是 %v 的方法", reflect.TypeOf(method), reflect.TypeOf(meta.Actor))
		return nil
	}

	// 创建Future
	future := context.RequestFuture(groupPID, &QueuedMsg{
		ID:              id,
		Method:          method,
		Params:          args,
		IsRequestFuture: true,
	}, timeout)

	return future
}

// todollw map获取的时候加读锁，需要看一下有没有更好的方案
// Send 发送消息，group下的actor通过group actor串行调度
func Send[T any](uniqueID string, method interface{}, args []interface{}) {
	actorFactory.mu.RLock()
	id := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[id]
	actorFactory.mu.RUnlock()
	if !ok {
		return
	}

	meta.Send(method, args)
}

// Stop 停止并移除指定 actor
func Stop[T any](uniqueID string) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	id := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[id]
	if !ok {
		return
	}
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
func StopGroup(group ActorGroup) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	ids, ok := actorFactory.groups[group]
	if !ok {
		return
	}
	for id := range ids {
		if meta, ok := actorFactory.actors[id]; ok {
			context.Stop(meta.PID)
			delete(actorFactory.actors, id)
		}
		_ = id
	}
	if groupPID, ok := actorFactory.groupPID[group]; ok {
		context.Stop(groupPID)
		delete(actorFactory.groupPID, group)
	}
	delete(actorFactory.groups, group)
}

// StopAll 停止并移除所有 actor
func StopAll() {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	for _, meta := range actorFactory.actors {
		context.Stop(meta.PID)
	}
	for _, groupPID := range actorFactory.groupPID {
		context.Stop(groupPID)
	}
	actorFactory.actors = make(map[string]*ActorMeta)
	actorFactory.groups = make(map[ActorGroup]map[string]struct{})
	actorFactory.groupPID = make(map[ActorGroup]*actor.PID)
}

func getUniqueName[T any](uniqueID string) string {
	return fmt.Sprintf("%s_%s", getName[T](), uniqueID)
}

func getName[T any]() string {
	var t T
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}
