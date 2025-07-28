package processor

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/modules/login/internal/models"
)

type BaseLoginProcessor interface {
	ReqLogin(context context.Context, req *message.C2S_Login) *models.LoginResponse
}
