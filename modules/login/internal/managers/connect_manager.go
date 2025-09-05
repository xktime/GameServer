package managers

import (
	"gameserver/common/base/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"sync"
	"time"
)

// 心跳超时时间（秒）
const HeartbeatTimeout = 60

// 客户端心跳信息
type ClientHeartbeat struct {
	LastHeartbeat time.Time
	Agent         gate.Agent
}

// ConnectManager 使用TaskHandler实现，确保连接管理操作按顺序执行
type ConnectManager struct {
	*actor.TaskHandler
	clients map[string]*ClientHeartbeat // clientID -> 心跳信息
}

var (
	connectManager     *ConnectManager
	connectManagerOnce sync.Once
)

func GetConnectManager() *ConnectManager {
	connectManagerOnce.Do(func() {
		connectManager = &ConnectManager{}
		connectManager.Init()
	})
	return connectManager
}

// Init 初始化ConnectManager
func (m *ConnectManager) Init() {
	// 初始化TaskHandler
	m.TaskHandler = actor.InitTaskHandler(actor.Login, "connect", m)
	m.TaskHandler.Start()

	// 初始化客户端映射
	m.clients = make(map[string]*ClientHeartbeat)
}

func (m *ConnectManager) OnTimer() {
	m.CheckHeartbeats()
}

func (m *ConnectManager) GetInterval() int {
	return 10
}

// Stop 停止ConnectManager
func (m *ConnectManager) Stop() {
	m.TaskHandler.Stop()
}

// UpdateHeartbeat 更新客户端心跳 - 异步执行
func (cm *ConnectManager) UpdateHeartbeat(agent gate.Agent) {
	cm.SendTask(func() *actor.Response {
		cm.doUpdateHeartbeat(agent)
		return nil
	})
}

// doUpdateHeartbeat 更新客户端心跳的同步实现
func (cm *ConnectManager) doUpdateHeartbeat(agent gate.Agent) {
	clientID := agent.RemoteAddr().String()

	cm.clients[clientID] = &ClientHeartbeat{
		LastHeartbeat: time.Now(),
		Agent:         agent,
	}

	log.Debug("更新客户端心跳: %s", clientID)
}

// RemoveClient 移除客户端 - 异步执行
func (cm *ConnectManager) RemoveClient(clientID string) {
	cm.SendTask(func() *actor.Response {
		cm.doRemoveClient(clientID)
		return nil
	})
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
	cm.SendTask(func() *actor.Response {
		cm.doCheckHeartbeats()
		return nil
	})
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

// GetActiveClients 获取活跃客户端数量 - 异步执行
func (cm *ConnectManager) GetActiveClients() int {
	response := cm.SendTask(func() *actor.Response {
		return &actor.Response{
			Result: []interface{}{len(cm.clients)},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if count, ok := response.Result[0].(int); ok {
			return count
		}
	}
	return 0
}

// GetAllClients 获取所有客户端信息（用于调试）- 异步执行
func (cm *ConnectManager) GetAllClients() map[string]*ClientHeartbeat {
	response := cm.SendTask(func() *actor.Response {
		// 返回副本，避免外部修改
		result := make(map[string]*ClientHeartbeat)
		for k, v := range cm.clients {
			result[k] = v
		}
		return &actor.Response{
			Result: []interface{}{result},
		}
	})

	if response != nil && len(response.Result) > 0 {
		if clients, ok := response.Result[0].(map[string]*ClientHeartbeat); ok {
			return clients
		}
	}
	return make(map[string]*ClientHeartbeat)
}
