package managers

import (
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	actor_manager "gameserver/core/actor"
	"gameserver/core/gate"
	"gameserver/core/log"

	"sync"

	"github.com/google/uuid"
	"gopkg.in/mgo.v2/bson"
)

type UserManager struct {
	actor_manager.ActorMessageHandler
}

var (
	userManager *UserManager
	userOnce    sync.Once
)

func GetUserManager() *UserManager {
	userOnce.Do(func() {
		meta, _ := actor_manager.Register[UserManager]("1", actor_manager.User)
		userManager = &UserManager{
			ActorMessageHandler: *actor_manager.NewActorMessageHandler(meta),
		}
	})
	return userManager
}

// todollw 根据openid和区服去取数据
// todollw 登录成功之后，需要去对应区服取数据，返回数据并给登录信息存到userdata
func (m *UserManager) DoLogin(agent gate.Agent, openId string, serverId int32) {
	user, err := mongodb.FindOne[models.User](bson.M{"openid": openId, "serverid": serverId})
	if err != nil {
		log.Error("DoLogin find user failed: %v", err)
		return
	}
	// todo 初始化玩家actor
	if user == nil {
		// 新注册流程
		user = &models.User{
			OpenId:   openId,
			ServerId: serverId,
			Platform: models.DouYin,
		}
	} else {
		// 老用户流程
	}
}

// 缓存实现需要开发者自行覆盖
var memCache = make(map[string]models.User)

// todollw 设置缓存
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
