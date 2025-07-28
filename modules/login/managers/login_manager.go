package managers

import (
	"gameserver/common/db/mongodb"
	"gameserver/common/models"
	"gameserver/core/gate"
	"gameserver/core/log"
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

type LoginManager struct {
}

var (
	loginManager    *LoginManager
	userManagerOnce sync.Once
)

func GetLoginManager() *LoginManager {
	userManagerOnce.Do(func() {
		loginManager = &LoginManager{}
	})
	return loginManager
}

// todollw 根据openid和区服去取数据
// todollw 登录成功之后，需要去对应区服取数据，返回数据并给登录信息存到userdata
func (m *LoginManager) DoLogin(agent gate.Agent, openId string, serverId int32) {
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
