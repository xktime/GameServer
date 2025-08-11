package room

import (
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/models"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// todo Room数据库清理
type Room struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	RoomId                            string  `bson:"_id"`
	RoomMembers                       []int64 `bson:"room_members"`
}

func (r Room) GetPersistId() interface{} {
	return r.RoomId
}

func CreateRoom(members []*models.MatchRequest) *Room {
	roomId := generateRoomId()
	// 提取成员的PlayerId，转换成[]int64
	memberIds := make([]int64, 0, len(members))
	for _, m := range members {
		memberIds = append(memberIds, m.PlayerId)
	}
	meta, _ := actor_manager.Register[Room](roomId, actor_manager.User, func(room *Room) {
		room.RoomMembers = memberIds
		room.RoomId = roomId
	})
	return meta.Actor
}

func (r *Room) SendRoomMessage(msg proto.Message) {
	for _, member := range r.RoomMembers {
		p := game.External.UserManager.DirectCaller.GetPlayer(member)
		if p == nil {
			log.Debug("玩家 %d 不在线", member)
			continue
		}
		p.SendToClient(msg)
	}
}

// generateRoomId 生成房间ID
func generateRoomId() string {
	return uuid.New().String()
}
