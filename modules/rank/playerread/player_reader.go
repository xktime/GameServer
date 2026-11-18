package playerread

import "context"

type PlayerSnapshot struct {
	Name      string
	AvatarURL string
	Level     int32
}

type PlayerReader interface {
	// FindOnline 返回在线 Player 的值快照；离线或在线数据不完整时返回 false, nil，查询失败保留错误。
	FindOnline(ctx context.Context, playerID int64) (PlayerSnapshot, bool, error)
}
