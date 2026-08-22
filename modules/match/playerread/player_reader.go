package playerread

type PlayerSnapshot struct {
	TeamID int64
}

type TeamSnapshot struct {
	MemberIDs []int64
}

type PlayerReader interface {
	// FindOnline 返回 Player 当前在线状态的值快照。
	FindOnline(playerID int64) (PlayerSnapshot, bool)
	// FindOnlineTeam 返回在线 Player 所属 Team 的值快照；MemberIDs 归调用方所有。
	FindOnlineTeam(playerID int64) (TeamSnapshot, bool)
	// FindRandomOnline 从当前在线 Player 中排除指定 ID 后选择一个候选者。
	FindRandomOnline(excludedPlayerIDs []int64) (int64, bool)
}
