package session

// PlayerSession 是一次在线期间可跨模块传递的不可变身份快照。
type PlayerSession struct {
	ID        uint64
	AccountID string
	PlayerID  int64
}

func FromData(data any) (PlayerSession, bool) {
	snapshot, ok := data.(PlayerSession)
	return snapshot, ok
}

// PlayerIDFrom 提取认证 Handler 需要的玩家 ID。
func PlayerIDFrom(data any) (int64, bool) {
	snapshot, ok := FromData(data)
	if !ok {
		return 0, false
	}
	return snapshot.PlayerID, true
}
