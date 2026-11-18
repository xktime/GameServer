package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/session"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/playersession"
	"sync"
	"time"
)

const (
	LoginTimeout     = 30 * time.Second
	HeartbeatTimeout = 60 * time.Second
)

// 客户端心跳信息
type ClientHeartbeat struct {
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	Agent         gate.ConnectionAgent
}

// ConnectManager 的连接状态由绑定的 Actor 队列串行访问。
type ConnectManager struct {
	sessions   *playersession.State
	actor      *connectActor
	definition *actor.Definition[*connectActor, string]
	mu         sync.RWMutex
	recoveryMu utils.KeyedMutex[string]
}

func (h *ConnectManager) currentActor() *connectActor {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.actor
}

// connectActor 的可变状态只允许在所属邮箱内访问。
type connectActor struct {
	actor.BaseActor
	clients  map[gate.ConnectionID]*ClientHeartbeat
	now      func() time.Time
	sessions *playersession.State
}

func NewConnectManager(ctx context.Context, scope *actor.Scope, sessions *playersession.State) (*ConnectManager, error) {
	return newConnectManager(ctx, scope, time.Now, sessions)
}

func newConnectManager(ctx context.Context, scope *actor.Scope, now func() time.Time, sessions *playersession.State) (*ConnectManager, error) {
	if sessions == nil {
		return nil, fmt.Errorf("login: Player Session state is nil")
	}
	definition, err := actor.Define(scope, func(context.Context, string) (*connectActor, error) {
		return &connectActor{
			clients:  make(map[gate.ConnectionID]*ClientHeartbeat),
			now:      now,
			sessions: sessions,
		}, nil
	}, actor.DefinitionOptions{QueueCapacity: 1024})
	if err != nil {
		return nil, err
	}
	instance, err := definition.GetOrCreate(ctx, "connect")
	if err != nil {
		return nil, err
	}
	return &ConnectManager{sessions: sessions, actor: instance, definition: definition}, nil
}

func (handle *ConnectManager) Register(agent gate.ConnectionAgent) {
	cm := handle.currentActor()

	if agent == nil {
		return
	}
	handle.sessions.Register(agent)
	connectedAt := cm.now()
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.clients[agent.ConnectionID()] = &ClientHeartbeat{
			ConnectedAt:   connectedAt,
			LastHeartbeat: connectedAt,
			Agent:         agent,
		}
		return nil
	}); err != nil {
		handle.sessions.Disconnect(agent.ConnectionID())
		log.Error("登记连接失败: %v", err)
	}
}

// BeginLogin performs only a short state transition, without entering the
// connection Actor or waiting for User/Player preparation.
func (handle *ConnectManager) BeginLogin(connectionID gate.ConnectionID, sequence uint32) (*playersession.Attempt, error) {
	return handle.sessions.Begin(connectionID, sequence)
}

func (handle *ConnectManager) OnTimer() {
	handle.CheckHeartbeats()
}

// UpdateHeartbeat 更新客户端心跳 - 异步执行
func (handle *ConnectManager) UpdateHeartbeat(connectionID gate.ConnectionID) {
	cm := handle.currentActor()

	heartbeatAt := cm.now()
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doUpdateHeartbeat(connectionID, heartbeatAt)
		return nil
	}); err != nil {
		log.Error("更新客户端心跳失败: %v", err)
	}
}

// doUpdateHeartbeat 更新客户端心跳的同步实现
func (cm *connectActor) doUpdateHeartbeat(connectionID gate.ConnectionID, heartbeatAt time.Time) {
	if client, exists := cm.clients[connectionID]; exists {
		client.LastHeartbeat = heartbeatAt
	}
}

// RemoveClient 立即使登录尝试失效，再异步移除心跳记录。
func (handle *ConnectManager) RemoveClient(clientID gate.ConnectionID) {
	cm := handle.currentActor()

	handle.sessions.Disconnect(clientID)
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doRemoveClient(clientID)
		return nil
	}); err != nil {
		log.Error("移除客户端心跳失败: %v", err)
	}
}

// doRemoveClient 移除客户端的同步实现
func (cm *connectActor) doRemoveClient(clientID gate.ConnectionID) {
	if _, exists := cm.clients[clientID]; exists {
		delete(cm.clients, clientID)
		log.Debug("移除客户端心跳: %d", clientID)
	}
}

// CheckHeartbeats 检查所有客户端的心跳 - 异步执行
func (handle *ConnectManager) CheckHeartbeats() {
	cm := handle.currentActor()

	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doCheckHeartbeats()
		return nil
	}); err != nil {
		log.Error("提交心跳检查失败: %v", err)
	}
}

// doCheckHeartbeats 检查所有客户端心跳的同步实现
func (cm *connectActor) doCheckHeartbeats() {
	now := cm.now()
	var clientsToRemove []gate.ConnectionID
	for clientID, heartbeat := range cm.clients {
		observed, authenticated := session.FromData(heartbeat.Agent.UserData())
		idleTime := now.Sub(heartbeat.LastHeartbeat)
		timedOut := authenticated && idleTime > HeartbeatTimeout
		if !authenticated {
			idleTime = now.Sub(heartbeat.ConnectedAt)
			timedOut = idleTime > LoginTimeout
		}
		if timedOut {
			if !cm.sessions.Expire(clientID, observed) {
				continue
			}
			log.Error("客户端心跳超时，准备断开连接: %d, 超时时间: %v",
				clientID, idleTime)

			// 断开连接
			heartbeat.Agent.Close()
			log.Debug("成功断开超时客户端连接: %d", clientID)

			clientsToRemove = append(clientsToRemove, clientID)
		}
	}

	// 移除已断开的客户端
	for _, clientID := range clientsToRemove {
		cm.doRemoveClient(clientID)
	}

	if len(clientsToRemove) > 0 {
		log.Debug("本次心跳检查断开 %d 个超时客户端", len(clientsToRemove))
	}
}

// GetActiveClients 同步读取活跃客户端数量。
func (handle *ConnectManager) GetActiveClients() int {
	cm := handle.currentActor()

	result, err := actor.Query(context.Background(), cm.Ref(), func(actor.Context) (int, error) {
		return len(cm.clients), nil
	})
	if err != nil {
		log.Error("获取活跃客户端数量失败: %v", err)
		return 0
	}
	return result
}

// GetAllClients 同步返回客户端信息快照（用于调试）。
func (handle *ConnectManager) GetAllClients() map[gate.ConnectionID]*ClientHeartbeat {
	cm := handle.currentActor()

	result, err := actor.Query(context.Background(), cm.Ref(), func(actor.Context) (map[gate.ConnectionID]*ClientHeartbeat, error) {
		// 返回副本，避免外部修改
		clients := make(map[gate.ConnectionID]*ClientHeartbeat)
		for k, v := range cm.clients {
			copyOfHeartbeat := *v
			clients[k] = &copyOfHeartbeat
		}
		return clients, nil
	})
	if err != nil {
		log.Error("获取所有客户端失败: %v", err)
		return make(map[gate.ConnectionID]*ClientHeartbeat)
	}
	return result
}
