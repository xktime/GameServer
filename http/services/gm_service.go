package services

import (
	"fmt"
	"gameserver/modules/game"
)

type Result int32

const (
	ResultSuccess       Result = 0
	ResultPlayerOffline Result = 1
	ResultItemNotFound  Result = 2
)

type GmService struct {
}

func NewGmService() *GmService {
	return &GmService{}
}

func (s *GmService) AddItem(playerId int64, itemId int32, count int32) Result {
	player := game.External.UserManager.GetPlayer(playerId)
	fmt.Println(player)
	return ResultSuccess
}
