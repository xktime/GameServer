package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/playersession"
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
	definition, err := actor.Define(scope, actor.Login, func(context.Context, string) (*ConnectManager, error) {
		return &ConnectManager{
			clients:  make(map[gate.ConnectionID]*ClientHeartbeat),
			now:      now,
			sessions: sessions,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return definition.GetOrCreate(ctx, "connect")
}

func (cm *ConnectManager) Register(agent gate.ConnectionAgent) {
	if agent == nil {
		return
	}
	cm.sessions.Register(agent)
	connectedAt := cm.now()
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.clients[agent.ConnectionID()] = &ClientHeartbeat{
			ConnectedAt:   connectedAt,
			LastHeartbeat: connectedAt,
			Agent:         agent,
		}
		return nil
	}); err != nil {
		cm.sessions.Disconnect(agent.ConnectionID())
		log.Error("登记连接失败: %v", err)
	}
}

// BeginLogin performs only a short state transition, without entering the
// connection Actor or waiting for User/Player preparation.
func (cm *ConnectManager) BeginLogin(connectionID gate.ConnectionID, sequence uint32) (*playersession.Attempt, error) {
	return cm.sessions.Begin(connectionID, sequence)
}

func (m *ConnectManager) OnTimer() {
	m.CheckHeartbeats()
}

// UpdateHeartbeat 更新客户端心跳 - 异步执行
func (cm *ConnectManager) UpdateHeartbeat(connectionID gate.ConnectionID) {
	heartbeatAt := cm.now()
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doUpdateHeartbeat(connectionID, heartbeatAt)
		return nil
	}); err != nil {
		log.Error("更新客户端心跳失败: %v", err)
	}
}

// doUpdateHeartbeat 更新客户端心跳的同步实现
func (cm *ConnectManager) doUpdateHeartbeat(connectionID gate.ConnectionID, heartbeatAt time.Time) {
	if client, exists := cm.clients[connectionID]; exists {
		client.LastHeartbeat = heartbeatAt
	}
}

// RemoveClient 立即使登录尝试失效，再异步移除心跳记录。
func (cm *ConnectManager) RemoveClient(clientID gate.ConnectionID) {
	cm.sessions.Disconnect(clientID)
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doRemoveClient(clientID)
		return nil
	}); err != nil {
		log.Error("移除客户端心跳失败: %v", err)
	}
}

// doRemoveClient 移除客户端的同步实现
func (cm *ConnectManager) doRemoveClient(clientID gate.ConnectionID) {
	if _, exists := cm.clients[clientID]; exists {
		delete(cm.clients, clientID)
		log.Debug("移除客户端心跳: %d", clientID)
	}
}

// CheckHeartbeats 检查所有客户端的心跳 - 异步执行
func (cm *ConnectManager) CheckHeartbeats() {
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doCheckHeartbeats()
		return nil
	}); err != nil {
		log.Error("提交心跳检查失败: %v", err)
	}
}

// doCheckHeartbeats 检查所有客户端心跳的同步实现
func (cm *ConnectManager) doCheckHeartbeats() {
	now := cm.now()
	var clientsToRemove []gate.ConnectionID
	// 复制一份客户端列表，避免在遍历时修改map
	clientsCopy := make(map[gate.ConnectionID]*ClientHeartbeat)
	for k, v := range cm.clients {
		clientsCopy[k] = v
	}

	for clientID, heartbeat := range clientsCopy {
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
func (cm *ConnectManager) GetActiveClients() int {
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (int, error) {
		return len(cm.clients), nil
	})
	if err != nil {
		log.Error("获取活跃客户端数量失败: %v", err)
		return 0
	}
	return result
}

// GetAllClients 同步返回客户端信息快照（用于调试）。
func (cm *ConnectManager) GetAllClients() map[gate.ConnectionID]*ClientHeartbeat {
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (map[gate.ConnectionID]*ClientHeartbeat, error) {
		// 返回副本，避免外部修改
		clients := make(map[gate.ConnectionID]*ClientHeartbeat)
		for k, v := range cm.clients {
			clients[k] = v
		}
		return clients, nil
	})
	if err != nil {
		log.Error("获取所有客户端失败: %v", err)
		return make(map[gate.ConnectionID]*ClientHeartbeat)
	}
	return result
}
