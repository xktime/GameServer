package actor_manager

import (
	"gameserver/common/utils"
	"gameserver/core/log"
	"reflect"
	"runtime"
	"strings"

	"github.com/asynkron/protoactor-go/actor"
)

// ActorMeta 用于描述 actor 的元信息
// 支持分组、标签
type ActorMeta struct {
	ID    string
	PID   *actor.PID
	Group ActorGroup
	Tags  map[string]struct{}
	Actor actor.Actor
}

func (m *ActorMeta) Send(method interface{}, args []interface{}) {
	actorFactory.mu.RLock()
	groupPID := actorFactory.groupPID[m.Group]
	actorFactory.mu.RUnlock()
	if groupPID == nil {
		return
	}
	if !m.checkMethod(method) {
		log.Error("Send: 传入的method: %v, 不是 %v 的方法", reflect.TypeOf(method), reflect.TypeOf(m.Actor))
		return
	}
	// 使用Send发送，确保消息按顺序到达GroupActor
	context.Send(groupPID, &QueuedMsg{ID: m.ID, Params: args, Method: method, IsRequestFuture: false})
}

// todollw 可以做缓存，避免每次都反射
func (m *ActorMeta) checkMethod(method interface{}) bool {
	if m.Actor != nil && method != nil {
		actorType := reflect.TypeOf(m.Actor)
		methodVal := reflect.ValueOf(method)
		if methodVal.Kind() == reflect.Func {
			methodName := runtime.FuncForPC(methodVal.Pointer()).Name()
			methodName = strings.TrimSuffix(methodName, "-fm")
			for i := 0; i < actorType.NumMethod(); i++ {
				meth := actorType.Method(i)
				funcName := runtime.FuncForPC(meth.Func.Pointer()).Name()
				if funcName == methodName {
					return true
				}
			}
		}
	}
	return false
}

type ActorMessageHandler struct {
	actor.Actor
	ActorMeta *ActorMeta
}

func NewActorMessageHandler(actorMeta *ActorMeta) *ActorMessageHandler {
	return &ActorMessageHandler{
		ActorMeta: actorMeta,
	}
}

func (h *ActorMessageHandler) Receive(ctx actor.Context) {
	h.handleMessage(ctx)
}

func (h *ActorMessageHandler) AddToActor(method interface{}, args []interface{}) {
	h.ActorMeta.Send(method, args)
}

func (h *ActorMessageHandler) GetActerMeta() *ActorMeta {
	return h.ActorMeta
}

func (h *ActorMessageHandler) handleMessage(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) == 0 {
		return
	}

	_, err := utils.CallMethodWithParams(msg[0], msg[1:]...)
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
	Method          interface{}   // 方法
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
	if next.Method == nil {
		log.Error("actor method is nil")
		return
	}
	msg := append([]interface{}{next.Method}, next.Params...)
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
