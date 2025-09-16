package processor

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
)

type BaseLoginProcessor interface {
	ReqLogin(agent gate.Agent, context context.Context, req *message.C2S_Login) *models.LoginResponse
}
