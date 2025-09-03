package actor_manager

import (
	"gameserver/common/utils"
	"gameserver/core/log"
	"reflect"
	"sync"
	"time"

	"github.com/asynkron/protoactor-go/actor"
)

// 因为manager的init是用模板生成的，需要加载的数据和初始化需要实现OnInitData
type ActorInit interface {
	OnInitData()
}

type ActorTimer interface {
	GetInterval() int
	OnTimer()
}

func OnTimer() {
	actorsSnapshot := make(map[string]interface{}, len(actorFactory.actors))
	actorFactory.mu.RLock()
	{
		// 创建副本，同时保存key和meta
		for key, meta := range actorFactory.actors {
			actorsSnapshot[key] = meta
		}
	}
	actorFactory.mu.RUnlock()
	now := time.Now()
	for _, meta := range actorsSnapshot {
		a := getActorByReflect(meta)
		if a == nil {
			continue
		}
		if timer, ok := a.(ActorTimer); ok {
			if now.Second()%timer.GetInterval() == 0 {
				timer.OnTimer()
			}
		}
	}
}

// ActorMeta 用于描述 actor 的元信息
// 支持分组、标签
type ActorMeta[T any] struct {
	ID          string
	PID         *actor.PID
	Group       ActorGroup
	Tags        map[string]struct{}
	Actor       *T
	methodCache sync.Map // 缓存方法存在性检查结果，使用sync.Map确保线程安全
}

func (m *ActorMeta[T]) AddToActor(methodName string, args []interface{}) {
	m.Send(methodName, args)
}

func (m *ActorMeta[T]) Send(methodName string, args []interface{}) {
	actorFactory.mu.RLock()
	groupPID := actorFactory.groupPID[m.Group]
	actorFactory.mu.RUnlock()
	if groupPID == nil {
		return
	}
	if !m.checkMethod(methodName) {
		log.Error("Send: 传入的method: %v, 不是 %v 的方法", methodName, reflect.TypeOf(m.Actor))
		return
	}
	if args == nil {
		args = make([]interface{}, 0)
	}
	args = append([]interface{}{m.Actor}, args...)
	// 使用Send发送，确保消息按顺序到达GroupActor
	context.Send(groupPID, &QueuedMsg{ID: m.ID, Params: args, MethodName: methodName, IsRequestFuture: false})
}

// checkMethod 检查Actor是否拥有指定名称的方法
// 使用sync.Map缓存避免重复反射操作，确保线程安全
func (m *ActorMeta[T]) checkMethod(methodName string) bool {
	// 检查缓存
	if exists, ok := m.methodCache.Load(methodName); ok {
		return exists.(bool)
	}

	// 缓存未命中，执行反射查找
	val := reflect.ValueOf(m.Actor)

	// 尝试在指针上查找
	method := val.MethodByName(methodName)
	if method.IsValid() {
		m.methodCache.Store(methodName, true)
		return true
	}

	// 尝试在值上查找
	if val.Kind() == reflect.Ptr {
		elemMethod := val.Elem().MethodByName(methodName)
		if elemMethod.IsValid() {
			m.methodCache.Store(methodName, true)
			return true
		}
	}

	// 方法不存在，缓存结果
	m.methodCache.Store(methodName, false)
	return false
}

type ActorMessageHandler struct {
	actor.Actor `bson:"-"`
}

func (h *ActorMessageHandler) Receive(ctx actor.Context) {
	h.handleMessage(ctx)
}

func (h *ActorMessageHandler) handleMessage(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		log.Error("msg最少需要两个参数")
		return
	}

	_, err := utils.CallMethodWithParams(msg[1], msg[0].(string), msg[2:]...)
	if err != nil {
		log.Error("CallMethodWithParams error: %v", err)
		return
	}
}

// ========== 新增/重构部分 ==========
// GroupActor 负责串行调度同group下所有子actor的消息

type QueuedMsg struct {
	ID              string        // 子actor id
	Params          []interface{} // 消息内容
	MethodName      string        // 方法
	IsRequestFuture bool          // 是否是RequestFuture消息
}

type RegisterChild struct {
	ID  string
	PID *actor.PID
}

type GroupActor struct {
	children map[string]*actor.PID // name -> 子actor PID
	msgQueue []QueuedMsg
}

func (g *GroupActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *RegisterChild:
		if g.children == nil {
			g.children = make(map[string]*actor.PID)
		}
		g.children[msg.ID] = msg.PID
	case *QueuedMsg:
		g.msgQueue = append(g.msgQueue, *msg)
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
	if len(g.msgQueue) == 0 {
		return
	}
	next := g.msgQueue[0]
	g.msgQueue = g.msgQueue[1:]
	if next.MethodName == "" {
		log.Error("actor method is nil")
		return
	}
	msg := append([]interface{}{next.MethodName}, next.Params...)
	if pid, ok := g.children[next.ID]; ok {
		// Future消息使用RequestWithCustomSender发送消息给子actor
		if next.IsRequestFuture {
			ctx.RequestWithCustomSender(pid, msg, ctx.Sender())
		} else {
			// Send消息，直接发送给子actor
			ctx.Send(pid, msg)
		}
	}
}
