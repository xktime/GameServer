package room

import (
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/log"
	"gameserver/modules/game"
	"time"

	"google.golang.org/protobuf/proto"
)

// 房间配置常量
const (
	MaxRoomLifetime = 30 * time.Minute // 房间最大存活时间：30分钟
)

// todo 玩家退出房间
// Room 房间结构
type Room struct {
	actor_manager.ActorMessageHandler `bson:"-"`
	RoomId                            int64         `bson:"_id"`
	RoomMembers                       []int64       `bson:"room_members"`
	TeamIds                           []int64       `bson:"team_ids"`
	CreateTime                        time.Time     `bson:"create_time"`  // 房间创建时间
	MaxLifetime                       time.Duration `bson:"max_lifetime"` // 房间最大存活时间
}

// CreateRoom 创建房间
func CreateRoom(playerIds []int64, teamIds []int64) *Room {
	roomId := generateRoomId()
	meta, _ := actor_manager.Register(roomId, actor_manager.Room, func(room *Room) {
		room.RoomMembers = playerIds
		room.RoomId = roomId
		room.CreateTime = time.Now()
		room.MaxLifetime = MaxRoomLifetime
		room.TeamIds = teamIds
		log.Debug("房间 %d 创建成功，包含 %d 个玩家，最大存活时间: %v",
			roomId, len(playerIds), room.MaxLifetime)
	})
	return meta.Actor
}

func (r *Room) GetInterval() int {
	return 10
}

func (r *Room) OnTimer() {
	CheckExpiration(r.RoomId)
}

// CheckExpiration 检查房间是否过期，如果过期则自动停止
func (r *Room) CheckExpiration() {
	if r.IsExpired() {
		log.Debug("房间 %d 已过期，开始自动停止", r.RoomId)
		r.Stop()
	}
}

// cleanup 清理房间资源
func (r *Room) cleanup() {
	// 通知所有玩家离开房间
	for _, teamId := range r.TeamIds {
		game.External.TeamManager.LeaveRoom(teamId)
	}

	// 清空房间成员列表
	r.RoomMembers = nil
	r.TeamIds = nil
	log.Debug("房间 %d 资源清理完成", r.RoomId)
}

// Stop 手动停止房间
func (r *Room) Stop() {
	log.Debug("房间 %d 手动停止", r.RoomId)

	// 通知所有玩家房间关闭（使用日志记录，避免消息类型依赖）
	log.Debug("房间 %d 手动关闭，通知所有玩家", r.RoomId)

	// 清理资源
	r.cleanup()

	// 停止Actor
	actor_manager.StopGroup(actor_manager.Room, r.RoomId)
}

// IsExpired 检查房间是否已过期
func (r *Room) IsExpired() bool {
	return time.Since(r.CreateTime) >= r.MaxLifetime
}

func (r *Room) SendRoomMessage(msg proto.Message) {
	for _, member := range r.RoomMembers {
		p := game.External.UserManager.GetPlayer(member)
		if p == nil {
			log.Debug("玩家 %d 不在线", member)
			continue
		}
		p.SendToClient(msg)
	}
}

func (r *Room) SendRoomMessageExceptSelf(msg proto.Message, selfId int64) {
	for _, member := range r.RoomMembers {
		if member == selfId {
			continue
		}
		p := game.External.UserManager.GetPlayer(member)
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
	r.SendRoomMessageExceptSelf(msg, playerId)
}

// generateRoomId 生成房间ID
func generateRoomId() int64 {
	return utils.FlakeId()
}
