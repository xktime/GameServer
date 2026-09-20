package provision

import (
	"gameserver/common/models"
	playermodel "gameserver/modules/game/internal/models/player"
	"time"
)

// Status 表示玩家创建意图的持久化状态。
type Status string

const (
	StatusPending Status = "pending"
	StatusApplied Status = "applied"
	StatusFailed  Status = "failed"
)

const CurrentVersion = 1

// PlayerData 保存恢复首次登录所需的玩家持久化数据，
// 无需依赖内存中的 Actor 实例。
type PlayerData struct {
	PlayerID   int64                   `bson:"player_id"`
	TeamID     int64                   `bson:"team_id"`
	TowerLevel int32                   `bson:"tower_level"`
	PlayerInfo *playermodel.PlayerInfo `bson:"player_info"`
}

// TeamData 是为新玩家创建的初始队伍投影。
type TeamData struct {
	TeamID    int64   `bson:"team_id"`
	LeaderID  int64   `bson:"leader_id"`
	MemberIDs []int64 `bson:"member_ids"`
	RoomID    string  `bson:"room_id"`
}

// Intent 是用于创建并持久化新用户、玩家和队伍的幂等请求，
// 自身也会持久化，并以 AccountID 作为业务幂等键。
type Intent struct {
	AccountID string `bson:"_id"`
	IntentID  string `bson:"intent_id"`
	Version   int    `bson:"version"`
	Status    Status `bson:"status"`
	Attempts  int    `bson:"attempts"`

	NextAttemptAt time.Time `bson:"next_attempt_at"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
	LastError     string    `bson:"last_error"`

	User   models.User `bson:"user"`
	Player PlayerData  `bson:"player"`
	Team   TeamData    `bson:"team"`
}

func (i Intent) GetPersistId() interface{} {
	return i.AccountID
}

// NewInitialData 构造与首次登录一致的初始状态。
func NewInitialData(user models.User, teamID int64) (PlayerData, TeamData) {
	return PlayerData{
			PlayerID: user.PlayerId,
			TeamID:   teamID,
			PlayerInfo: &playermodel.PlayerInfo{
				PlayerId:      user.PlayerId,
				ServerId:      user.ServerId,
				PlayerName:    user.OpenId,
				Level:         1,
				Balance:       0,
				TotalRecharge: 0,
				VipLevel:      0,
			},
		}, TeamData{
			TeamID:    teamID,
			LeaderID:  user.PlayerId,
			MemberIDs: []int64{user.PlayerId},
		}
}
