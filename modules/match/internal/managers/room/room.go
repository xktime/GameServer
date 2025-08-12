package room

import (
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game"
	"gameserver/modules/match/internal/models"

	"google.golang.org/protobuf/proto"
)

// todo roomId需要改成雪花
// todo Room数据库清理, 比赛结束时手动清理
type Room struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	RoomId                            int64   `bson:"_id"`
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
	meta, _ := actor_manager.Register(roomId, actor_manager.User, func(room *Room) {
		room.RoomMembers = memberIds
		room.RoomId = roomId
	})
	return meta.Actor
}

// todo  应该是给其他人广播，需要排除自己
// todo 玩家掉线需要通知对方
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

func (r *Room) PlayerOffline(playerId int64) {
	msg := &message.S2C_PlayerOffline{
		PlayerId: playerId,
	}
	r.SendRoomMessage(msg)
}

// generateRoomId 生成房间ID
func generateRoomId() int64 {
	return utils.FlakeId()
}
