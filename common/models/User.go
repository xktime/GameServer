package models

import "gameserver/common/msg/message"

type Platform int32

type User struct {
	AccountId       string            `bson:"_id"`
	ServerId        int32             `bson:"ServerId" default:"0"`
	OpenId          string            `bson:"OpenId" default:""`
	PlayerId        int64             `bson:"PlayerId" default:"0"`
	Platform        message.LoginType `bson:"Platform" default:"0"`
	LastOfflineTime int64             `bson:"LastOfflineTime" default:"0"`
	LoginTime       int64             `bson:"LoginTime" default:"0"`
}

func (u User) GetPersistId() interface{} {
	return u.AccountId
}
