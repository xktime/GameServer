package managers

import (
	"gameserver/common/schedule"
	actor_manager "gameserver/core/actor"
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

// ConnectManager 连接管理器
type ConnectManager struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	clients                           map[string]*ClientHeartbeat // clientID -> 心跳信息
}

func (m *ConnectManager) OnInitData() {
	log.Debug("初始化连接管理器")
	m.clients = make(map[string]*ClientHeartbeat)
	schedule.RegisterIntervalSchedul(10, GetConnectManager().CheckHeartbeats)
}

// UpdateHeartbeat 更新客户端心跳
func (cm *ConnectManager) UpdateHeartbeat(agent gate.Agent) {
	clientID := agent.RemoteAddr().String()

	cm.clients[clientID] = &ClientHeartbeat{
		LastHeartbeat: time.Now(),
		Agent:         agent,
	}

	log.Debug("更新客户端心跳: %s", clientID)
}

// RemoveClient 移除客户端
func (cm *ConnectManager) RemoveClient(clientID string) {
	if _, exists := cm.clients[clientID]; exists {
		delete(cm.clients, clientID)
		log.Debug("移除客户端心跳: %s", clientID)
	}
}

// checkHeartbeats 检查所有客户端的心跳
func (cm *ConnectManager) CheckHeartbeats() {
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
		cm.RemoveClient(clientID)
	}

	if len(clientsToRemove) > 0 {
		log.Debug("本次心跳检查断开 %d 个超时客户端", len(clientsToRemove))
	}
}

// GetActiveClients 获取活跃客户端数量
func (cm *ConnectManager) GetActiveClients() int {
	return len(cm.clients)
}

// GetAllClients 获取所有客户端信息（用于调试）
func (cm *ConnectManager) GetAllClients() map[string]*ClientHeartbeat {
	// 返回副本，避免外部修改
	result := make(map[string]*ClientHeartbeat)
	for k, v := range cm.clients {
		result[k] = v
	}
	return result
}
