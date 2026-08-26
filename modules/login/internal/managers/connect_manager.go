package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"time"
)

// 心跳超时时间（秒）
const HeartbeatTimeout = 60

// 客户端心跳信息
type ClientHeartbeat struct {
	LastHeartbeat time.Time
	Agent         gate.Agent
}

// ConnectManager 的连接状态由绑定的 Actor 队列串行访问。
type ConnectManager struct {
	actor.BaseActor
	clients map[string]*ClientHeartbeat // clientID -> 心跳信息
}

func NewConnectManager(ctx context.Context, scope *actor.Scope) (*ConnectManager, error) {
	definition, err := actor.Define(scope, actor.Login, func(context.Context, string) (*ConnectManager, error) {
		return &ConnectManager{clients: make(map[string]*ClientHeartbeat)}, nil
	})
	if err != nil {
		return nil, err
	}
	return definition.GetOrCreate(ctx, "connect")
}

func (m *ConnectManager) OnTimer() {
	m.CheckHeartbeats()
}

// UpdateHeartbeat 更新客户端心跳 - 异步执行
func (cm *ConnectManager) UpdateHeartbeat(agent gate.Agent) {
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doUpdateHeartbeat(agent)
		return nil
	}); err != nil {
		log.Error("更新客户端心跳失败: %v", err)
	}
}

// doUpdateHeartbeat 更新客户端心跳的同步实现
func (cm *ConnectManager) doUpdateHeartbeat(agent gate.Agent) {
	clientID := agent.RemoteAddr().String()

	cm.clients[clientID] = &ClientHeartbeat{
		LastHeartbeat: time.Now(),
		Agent:         agent,
	}
}

// RemoveClient 移除客户端 - 异步执行
func (cm *ConnectManager) RemoveClient(clientID string) {
	if err := actor.Tell(context.Background(), cm.Ref(), func(actor.Context) error {
		cm.doRemoveClient(clientID)
		return nil
	}); err != nil {
		log.Error("移除客户端心跳失败: %v", err)
	}
}

// doRemoveClient 移除客户端的同步实现
func (cm *ConnectManager) doRemoveClient(clientID string) {
	if _, exists := cm.clients[clientID]; exists {
		delete(cm.clients, clientID)
		log.Debug("移除客户端心跳: %s", clientID)
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
	now := time.Now()
	var clientsToRemove []string
	// 复制一份客户端列表，避免在遍历时修改map
	clientsCopy := make(map[string]*ClientHeartbeat)
	for k, v := range cm.clients {
		clientsCopy[k] = v
	}

	for clientID, heartbeat := range clientsCopy {
		// 检查是否超时
		if now.Sub(heartbeat.LastHeartbeat) > HeartbeatTimeout*time.Second {
			log.Error("客户端心跳超时，准备断开连接: %s, 超时时间: %v",
				clientID, now.Sub(heartbeat.LastHeartbeat))

			// 断开连接
			heartbeat.Agent.Close()
			log.Debug("成功断开超时客户端连接: %s", clientID)

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
func (cm *ConnectManager) GetAllClients() map[string]*ClientHeartbeat {
	result, err := actor.Call(context.Background(), cm.Ref(), func(actor.Context) (map[string]*ClientHeartbeat, error) {
		// 返回副本，避免外部修改
		clients := make(map[string]*ClientHeartbeat)
		for k, v := range cm.clients {
			clients[k] = v
		}
		return clients, nil
	})
	if err != nil {
		log.Error("获取所有客户端失败: %v", err)
		return make(map[string]*ClientHeartbeat)
	}
	return result
}
