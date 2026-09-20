package network

import "gameserver/core/network/models"

type Processor interface {
	// 实现必须支持并发调用。
	Route(msg *models.MsgWithSeq, userData interface{}) error
	// 实现必须支持并发调用。
	Unmarshal(data []byte) (*models.MsgWithSeq, error)
	// 实现必须支持并发调用。
	Marshal(msg interface{}, seq uint32) ([][]byte, error)
}
