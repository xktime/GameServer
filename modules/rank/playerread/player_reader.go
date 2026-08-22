package playerread

type PlayerSnapshot struct {
	Name      string
	AvatarURL string
	Level     int32
}

type PlayerReader interface {
	// FindOnline 返回在线 Player 的值快照；离线或在线数据不完整时返回 false。
	FindOnline(playerID int64) (PlayerSnapshot, bool)
}
