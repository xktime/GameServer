package participant

// DesiredTeamRoom 表示一个真实队伍期望同步的最新房间投影。
// RoomID 为空时清除该房间投影。
type DesiredTeamRoom struct {
	TeamID int64
	RoomID string
}

// TeamRoomProjection 以幂等方式更新队伍的房间投影。
type TeamRoomProjection interface {
	Apply(desired []DesiredTeamRoom) error
}
