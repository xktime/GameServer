package models

type Platform int32

const (
	DouYin Platform = 1
	WeChat Platform = 2
)

type User struct {
	AccountId       string   `bson:"_id"`
	ServerId        int32    `bson:"ServerId" default:"0"`
	OpenId          string   `bson:"OpenId" default:""`
	PlayerId        int64    `bson:"PlayerId" default:"0"`
	Platform        Platform `bson:"Platform" default:"0"`
	LastOfflineTime int64    `bson:"LastOfflineTime" default:"0"`
}
