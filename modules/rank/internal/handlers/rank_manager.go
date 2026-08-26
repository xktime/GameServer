package handlers

import "gameserver/common/msg/message"

type RankManager interface {
	HandleGetRankList(int64, *message.C2S_GetRankList) *message.S2C_GetRankList
	HandleGetMyRank(int64, message.RankType, int32) *message.S2C_GetMyRank
	HandleUpdateRankData(int64, *message.C2S_UpdateRankData) *message.S2C_UpdateRankData
	GetSeasonInfo() *message.S2C_SeasonInfo
	GeneratorChallengeCode(*message.C2S_GeneratorChanllengeCode) *message.S2C_GeneratorChanllengeCode
	ChanllengeByCode(*message.C2S_ChanllengeByCode) *message.S2C_ChanllengeByCode
	GetChallengeList(int64, *message.C2S_GetChanllengeList) *message.S2C_GetChanllengeList
	GetMyHistoryRank(int64) *message.S2C_GetMyHistoryRank
	GetHistoryRankReward(int64, *message.C2S_GetHistoryRankReward) *message.S2C_GetHistoryRankReward
}
