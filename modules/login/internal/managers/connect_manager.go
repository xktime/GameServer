package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/core/log"
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
	LoginAttempt  uint64
}

// LoginAttempt 标识某个连接的一次登录请求。序号在管理器内全局递增，
// 因此即使重连复用了连接编号，也不会接收此前请求的过期响应。
type LoginAttempt struct {
	ConnectionID gate.ConnectionID
	Sequence     uint64
}

// ConnectManager 的连接状态由绑定的 Actor 队列串行访问。
type ConnectManager struct {
	actor.BaseActor
	clients  map[gate.ConnectionID]*ClientHeartbeat
	now      func() time.Time
	sequence uint64
}

func NewConnectManager(ctx context.Context, scope *actor.Scope) (*ConnectManager, error) {
	return newConnectManager(ctx, scope, time.Now)
}

func newConnectManager(ctx context.Context, scope *actor.Scope, now func() time.Time) (*ConnectManager, error) {
	definition, err := actor.Define(scope, actor.Login, func(context.Context, string) (*ConnectManager, error) {
		return &ConnectManager{
			clients: make(map[gate.ConnectionID]*ClientHeartbeat),
			now:     now,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	return definition.GetOrCreate(ctx, "connect")
}

func (cm *ConnectManager) Register(agent gate.ConnectionAgent) {
	connectedAt := cm.now()
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.clients[agent.ConnectionID()] = &ClientHeartbeat{
			ConnectedAt:   connectedAt,
			LastHeartbeat: connectedAt,
			Agent:         agent,
		}
		return nil
	}); err != nil {
		log.Error("登记连接失败: %v", err)
	}
}

// BeginLogin 为已注册的连接推进登录请求代次。
func (cm *ConnectManager) BeginLogin(connectionID gate.ConnectionID) (LoginAttempt, error) {
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (LoginAttempt, error) {
		client, exists := cm.clients[connectionID]
		if !exists {
			return LoginAttempt{}, fmt.Errorf("login connection %d is not registered", connectionID)
		}
		cm.sequence++
		client.LoginAttempt = cm.sequence
		return LoginAttempt{ConnectionID: connectionID, Sequence: cm.sequence}, nil
	})
	if err != nil {
		return LoginAttempt{}, err
	}
	return result, nil
}

// IsCurrentLoginAttempt 检查响应是否仍属于当前连接的最新登录请求；
// 旧请求及连接移除后重新注册前的请求均视为过期。
func (cm *ConnectManager) IsCurrentLoginAttempt(attempt LoginAttempt) bool {
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (bool, error) {
		client, exists := cm.clients[attempt.ConnectionID]
		return exists && client.LoginAttempt == attempt.Sequence, nil
	})
	if err != nil {
		log.Error("校验登录请求代际失败: %v", err)
		return false
	}
	return result
}

// WithCurrentLogin runs fn while the connection actor holds the current
// generation check, so a newer login request cannot start between session
// creation and response delivery.
func (cm *ConnectManager) WithCurrentLogin(attempt LoginAttempt, fn func()) bool {
	if fn == nil {
		return false
	}
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (bool, error) {
		client, exists := cm.clients[attempt.ConnectionID]
		if !exists || client.LoginAttempt != attempt.Sequence {
			return false, nil
		}
		fn()
		return true, nil
	})
	if err != nil {
		log.Error("执行当前登录请求失败: %v", err)
		return false
	}
	return result
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

// RemoveClient 移除客户端 - 异步执行
func (cm *ConnectManager) RemoveClient(clientID gate.ConnectionID) {
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
		_, authenticated := session.FromData(heartbeat.Agent.UserData())
		idleTime := now.Sub(heartbeat.LastHeartbeat)
		timedOut := authenticated && idleTime > HeartbeatTimeout
		if !authenticated {
			idleTime = now.Sub(heartbeat.ConnectedAt)
			timedOut = idleTime > LoginTimeout
		}
		if timedOut {
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
