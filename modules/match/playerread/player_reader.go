package playerread

import "context"

type PlayerSnapshot struct {
	TeamID int64
}

type TeamSnapshot struct {
	TeamID    int64
	MemberIDs []int64
}

type PlayerReader interface {
	// FindOnline 返回 Player 当前在线状态的值快照；离线返回 false, nil，查询失败保留错误。
	FindOnline(ctx context.Context, playerID int64) (PlayerSnapshot, bool, error)
	// FindOnlineTeam 返回在线 Player 所属 Team 的值快照；MemberIDs 归调用方所有，查询失败保留错误。
	FindOnlineTeam(ctx context.Context, playerID int64) (TeamSnapshot, bool, error)
}
