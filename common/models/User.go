package models

type Platform int32

const (
	DouYin Platform = 1
	WeChat Platform = 2
)

type User struct {
	AccountId       string `bson:"_id"`
	ServerId        int32
	OpenId          string
	PlayerId        int64
	Platform        Platform
	LastOfflineTime int64
}
