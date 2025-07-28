package actor_manager

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
	"google.golang.org/protobuf/proto"
)

// ActorMeta 用于描述 actor 的元信息
// 支持分组、标签
type ActorMeta struct {
	PID   *actor.PID
	Group ActorGroup
	Tags  map[string]struct{}
}

// ========== 新增/重构部分 ==========
// GroupActor 负责串行调度同group下所有子actor的消息

type QueuedMsg struct {
	Name            string      // 子actor name
	Msg             interface{} // 消息内容
	IsRequestFuture bool        // 是否是RequestFuture消息
}

type RegisterChild struct {
	Name string
	PID  *actor.PID
}

type GroupActor struct {
	children map[string]*actor.PID // name -> 子actor PID
	msgQueue []QueuedMsg
	mu       sync.Mutex // 保护msgQueue的访问
}

func (g *GroupActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *RegisterChild:
		if g.children == nil {
			g.children = make(map[string]*actor.PID)
		}
		g.children[msg.Name] = msg.PID
	case *QueuedMsg:
		g.mu.Lock()
		g.msgQueue = append(g.msgQueue, *msg)
		g.mu.Unlock()
		g.tryDispatch(ctx)
	default:
		// 如果发送者不是nil，说明这是对RequestFuture的响应
		if ctx.Sender() != nil {
			ctx.Respond(ctx.Message())
		}
		// 处理完一个消息后，继续处理队列
		g.tryDispatch(ctx)
	}
}

func (g *GroupActor) tryDispatch(ctx actor.Context) {
	g.mu.Lock()
	if len(g.msgQueue) == 0 {
		g.mu.Unlock()
		return
	}
	next := g.msgQueue[0]
	g.msgQueue = g.msgQueue[1:]
	g.mu.Unlock()

	if pid, ok := g.children[next.Name]; ok {
		// Future消息使用RequestWithCustomSender发送消息给子actor
		if next.IsRequestFuture {
			ctx.RequestWithCustomSender(pid, next.Msg, ctx.Sender())
		} else {
			// Send消息，直接发送给子actor
			ctx.Send(pid, next.Msg)
		}
	}
}

// ActorFactory 用于创建和管理 actor
// 支持分组、标签、批量操作
type ActorFactory struct {
	mu       sync.RWMutex
	actors   map[string]*ActorMeta              // name -> meta
	groups   map[ActorGroup]map[string]struct{} // group -> set of names
	tags     map[string]map[string]struct{}     // tag -> set of names
	groupPID map[ActorGroup]*actor.PID          // group -> group actor PID
}

// NewActorFactory 创建一个新的 ActorFactory
func NewActorFactory() *ActorFactory {
	return &ActorFactory{
		actors:   make(map[string]*ActorMeta),
		groups:   make(map[ActorGroup]map[string]struct{}),
		tags:     make(map[string]map[string]struct{}),
		groupPID: make(map[ActorGroup]*actor.PID),
	}
}

var (
	actorFactory *ActorFactory
	system       *actor.ActorSystem
	context      *actor.RootContext
	timeout      time.Duration
)

func Init(milliseconds int) {
	actorFactory = NewActorFactory()
	system = actor.NewActorSystem()
	context = system.Root
	timeout = time.Duration(milliseconds) * time.Millisecond
}

// Register 注册并启动一个 actor
func Register[T any](uniqueID string, group ActorGroup, actorImpl actor.Actor, tags ...string) (*actor.PID, error) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	name := getUniqueName[T](uniqueID)
	if _, exists := actorFactory.actors[name]; exists {
		return nil, nil // 已存在
	}

	// 1. group actor 不存在则创建
	var groupPID *actor.PID
	if group != "" {
		if existingPID, ok := actorFactory.groupPID[group]; ok {
			groupPID = existingPID
		} else {
			props := actor.PropsFromProducer(func() actor.Actor {
				return &GroupActor{}
			})
			groupPID = context.Spawn(props)
			actorFactory.groupPID[group] = groupPID
		}
	}

	// 2. 创建子actor
	props := actor.PropsFromProducer(func() actor.Actor {
		return actorImpl
	})
	pid := context.Spawn(props)

	// 3. 注册子actor到group actor
	if groupPID != nil {
		context.Send(groupPID, &RegisterChild{Name: name, PID: pid})
	}

	meta := &ActorMeta{
		PID:   pid,
		Group: group,
		Tags:  make(map[string]struct{}),
	}
	for _, tag := range tags {
		meta.Tags[tag] = struct{}{}
		if _, ok := actorFactory.tags[tag]; !ok {
			actorFactory.tags[tag] = make(map[string]struct{})
		}
		actorFactory.tags[tag][name] = struct{}{}
	}
	actorFactory.actors[name] = meta
	if group != "" {
		if _, ok := actorFactory.groups[group]; !ok {
			actorFactory.groups[group] = make(map[string]struct{})
		}
		actorFactory.groups[group][name] = struct{}{}
	}
	return pid, nil
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
	name := getUniqueName[T](uniqueID)
	if meta, ok := actorFactory.actors[name]; ok {
		return meta.PID
	}
	return nil
}

func RequestFuture[T any](uniqueID string, message proto.Message) *actor.Future {
	actorFactory.mu.RLock()
	name := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[name]
	actorFactory.mu.RUnlock()
	if !ok {
		return nil
	}

	// 如果actor属于某个group，通过group actor队列执行
	if meta.Group != "" {
		actorFactory.mu.RLock()
		groupPID := actorFactory.groupPID[meta.Group]
		actorFactory.mu.RUnlock()
		if groupPID == nil {
			return nil
		}

		// 创建Future
		future := context.RequestFuture(groupPID, &QueuedMsg{
			Name:            name,
			Msg:             message,
			IsRequestFuture: true,
		}, timeout)

		return future
	}

	// 不属于group的actor直接发送
	return context.RequestFuture(meta.PID, message, timeout)
}

// Send 发送消息，group下的actor通过group actor串行调度
func Send[T any](uniqueID string, message proto.Message) {
	actorFactory.mu.RLock()
	name := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[name]
	actorFactory.mu.RUnlock()
	if !ok {
		return
	}
	if meta.Group != "" {
		actorFactory.mu.RLock()
		groupPID := actorFactory.groupPID[meta.Group]
		actorFactory.mu.RUnlock()
		if groupPID == nil {
			return
		}
		// 使用Send发送，确保消息按顺序到达GroupActor
		context.Send(groupPID, &QueuedMsg{Name: name, Msg: message, IsRequestFuture: false})
		return
	}
	context.Send(meta.PID, message)
}

// Stop 停止并移除指定 actor
func Stop[T any](uniqueID string) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	name := getUniqueName[T](uniqueID)
	meta, ok := actorFactory.actors[name]
	if !ok {
		return
	}
	context.Stop(meta.PID)
	delete(actorFactory.actors, name)
	if meta.Group != "" {
		delete(actorFactory.groups[meta.Group], name)
		if len(actorFactory.groups[meta.Group]) == 0 {
			// group下无子actor时，停止group actor
			if groupPID, ok := actorFactory.groupPID[meta.Group]; ok {
				context.Stop(groupPID)
				delete(actorFactory.groupPID, meta.Group)
			}
			delete(actorFactory.groups, meta.Group)
		}
	}
	for tag := range meta.Tags {
		delete(actorFactory.tags[tag], name)
		if len(actorFactory.tags[tag]) == 0 {
			delete(actorFactory.tags, tag)
		}
	}
}

// StopGroup 停止并移除某个分组下的所有 actor
func StopGroup(group ActorGroup) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	names, ok := actorFactory.groups[group]
	if !ok {
		return
	}
	for name := range names {
		if meta, ok := actorFactory.actors[name]; ok {
			context.Stop(meta.PID)
			delete(actorFactory.actors, name)
			for tag := range meta.Tags {
				delete(actorFactory.tags[tag], name)
				if len(actorFactory.tags[tag]) == 0 {
					delete(actorFactory.tags, tag)
				}
			}
		}
		_ = name
	}
	if groupPID, ok := actorFactory.groupPID[group]; ok {
		context.Stop(groupPID)
		delete(actorFactory.groupPID, group)
	}
	delete(actorFactory.groups, group)
}

// StopByTag 停止并移除带有某个标签的所有 actor
func StopByTag(tag string) {
	actorFactory.mu.Lock()
	defer actorFactory.mu.Unlock()
	names, ok := actorFactory.tags[tag]
	if !ok {
		return
	}
	for name := range names {
		if meta, ok := actorFactory.actors[name]; ok {
			context.Stop(meta.PID)
			delete(actorFactory.actors, name)
			if meta.Group != "" {
				delete(actorFactory.groups[meta.Group], name)
				if len(actorFactory.groups[meta.Group]) == 0 {
					delete(actorFactory.groups, meta.Group)
				}
			}
		}
		// 确保 name 被使用
		_ = name
	}
	delete(actorFactory.tags, tag)
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
	actorFactory.tags = make(map[string]map[string]struct{})
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
