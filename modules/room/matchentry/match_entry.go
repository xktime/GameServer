package matchentry

// MatchedTeam 是匹配模块传递给房间模块的不可变队伍快照。
type MatchedTeam struct {
	TeamID    int64
	PlayerIDs []int64
	IsRobot   bool
}

// Admission 是将一组匹配成功的队伍转为房间的幂等请求。
type Admission struct {
	MatchID string
	Teams   []MatchedTeam
}

// AcceptanceStatus 定义房间接纳请求的所有可能结果。
type AcceptanceStatus uint8

const (
	Accepted AcceptanceStatus = iota + 1
	AlreadyAccepted
	Rejected
	Retryable
)

// Acceptance 表示接纳一组匹配队伍后可供调用方观察的结果。
type Acceptance struct {
	Status          AcceptanceStatus
	RoomID          string
	RejectedTeamIDs []int64
}
