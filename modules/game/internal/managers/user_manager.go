package managers

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"

	"sync"

	"github.com/google/uuid"
	"gopkg.in/mgo.v2/bson"
)

type UserManager struct {
	actor_manager.ActorMessageHandler
	//	actorMeta actor_manager.ActorMeta[UserManager]
}

var (
	meta     *actor_manager.ActorMeta[UserManager]
	userOnce sync.Once
)

func GetUserManager() *UserManager {
	userOnce.Do(func() {
		meta, _ = actor_manager.Register[UserManager]("1", actor_manager.User)
	})
	return meta.Actor
}

func (m *UserManager) DoLoginByActor(agent gate.Agent, openId string, serverId int32) {
	meta.AddToActor("DoLogin", []interface{}{agent, openId, serverId})
}

func (m *UserManager) DoLogin(agent gate.Agent, openId string, serverId int32) {
	user, err := mongodb.FindOne[models.User](bson.M{"OpenId": openId, "ServerId": serverId})
	if err != nil {
		log.Error("DoLogin find user failed: %v", err)
		return
	}

	isNew := user == nil
	if isNew {
		// 新注册流程
		user = &models.User{
			AccountId: fmt.Sprintf("%d_%s", serverId, openId),
			OpenId:    openId,
			ServerId:  serverId,
			PlayerId:  utils.FlakeId(),
			// todo platform需要传进来修改？
			Platform: models.DouYin,
		}
		if _, err := mongodb.Save(user); err != nil {
			log.Error("Failed to save new user [openId: %s, serverId: %d]: %v", openId, serverId, err)
			return
		}
		log.Debug("DoLogin new user: %v", user)
	} else {
		// 老用户流程
		log.Debug("DoLogin old user: %v", user)
	}
	agent.SetUserData(*user)
	player.Login(agent, isNew)
}

var memCache = make(map[string]models.User)

// todo 设置缓存
func (m *UserManager) SetUserCache() {
	// cacheData, exist := memCache[token]
	// if !exist {
	// 	newToken := m.GenToken(context)
	// } else {
	// 	memCache[token] = cacheData
	// }
}

func (m *UserManager) GetCache(accountId string) (models.User, bool) {
	data, exist := memCache[accountId]
	return data, exist
}

func (m *UserManager) GenToken() string {
	return uuid.New().String()
}
