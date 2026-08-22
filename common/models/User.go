package models

import (
	"gameserver/common/msg/message"
	"gameserver/common/utils"
)

type Platform int32

type User struct {
	AccountId       string            `bson:"_id"`
	OpenId          string            `bson:"OpenId" default:""`
	PlayerId        int64             `bson:"PlayerId" default:"0"`
	LastOfflineTime int64             `bson:"LastOfflineTime" default:"0"`
	LoginTime       int64             `bson:"LoginTime" default:"0"`
	CreateTime      int64             `bson:"CreateTime" default:"0"`
	ServerId        int32             `bson:"ServerId" default:"0"`
	Platform        message.LoginType `bson:"Platform" default:"0"`
	TotalLoginDays  int32             `bson:"TotalLoginDays" default:"0"`
}

func (u User) GetPersistId() interface{} {
	return u.AccountId
}

// RecordLogin 记录一次登录，同一自然日内重复调用不会重复增加总登录天数。
func (u *User) RecordLogin(now int64) {
	if u.TotalLoginDays == 0 {
		u.TotalLoginDays = 1
	} else {
		lastActiveTime := max(u.LastOfflineTime, u.LoginTime)
		if utils.IsCrossDay(lastActiveTime, now) {
			u.TotalLoginDays++
		}
	}
	u.LoginTime = now
}
