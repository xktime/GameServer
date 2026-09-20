package managers

import (
	"fmt"
	"gameserver/common/models"
	"gameserver/common/session"
	"gameserver/core/gate"
)

type activePlayerSession struct {
	snapshot session.PlayerSession
	user     *models.User
	agent    gate.Agent
}

// playerSessions 保存账号当前的在线会话，由 UserManager 的 Actor 串行访问。
type playerSessions struct {
	nextID    uint64
	byAccount map[string]activePlayerSession
}

func newPlayerSessions() *playerSessions {
	return &playerSessions{byAccount: make(map[string]activePlayerSession)}
}

// open 为账号创建唯一的在线会话，并分配当前会话表内单调递增的会话编号。
func (s *playerSessions) open(user *models.User, agent gate.Agent) (activePlayerSession, error) {
	if _, exists := s.byAccount[user.AccountId]; exists {
		return activePlayerSession{}, fmt.Errorf("Player Session 已存在: %s", user.AccountId)
	}
	s.nextID++
	active := activePlayerSession{
		snapshot: session.PlayerSession{
			ID:        s.nextID,
			AccountID: user.AccountId,
			PlayerID:  user.PlayerId,
		},
		user:  user,
		agent: agent,
	}
	s.byAccount[user.AccountId] = active
	return active, nil
}

func (s *playerSessions) current(accountID string) (activePlayerSession, bool) {
	active, ok := s.byAccount[accountID]
	return active, ok
}

// close 仅移除与快照完全匹配的会话，防止旧连接的下线通知关闭新会话。
func (s *playerSessions) close(snapshot session.PlayerSession) (activePlayerSession, bool) {
	active, ok := s.byAccount[snapshot.AccountID]
	if !ok || active.snapshot != snapshot {
		return activePlayerSession{}, false
	}
	delete(s.byAccount, snapshot.AccountID)
	return active, true
}

func (s *playerSessions) all() []activePlayerSession {
	active := make([]activePlayerSession, 0, len(s.byAccount))
	for _, current := range s.byAccount {
		active = append(active, current)
	}
	return active
}
