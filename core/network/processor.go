package network

import "gameserver/core/network/models"

type Processor interface {
	// must goroutine safe
	Route(msg *models.MsgWithSeq, userData interface{}) error
	// must goroutine safe
	Unmarshal(data []byte) (*models.MsgWithSeq, error)
	// must goroutine safe
	Marshal(msg interface{}, seq uint32) ([][]byte, error)
}
