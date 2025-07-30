package processor

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/login/internal/models"
	"strconv"

	"github.com/google/uuid"
)

type WeChatLoginProcessor struct {
}

func NewWechatLoginProcessor() *WeChatLoginProcessor {
	return &WeChatLoginProcessor{}
}

func (p *WeChatLoginProcessor) ReqLogin(context context.Context, req *message.C2S_Login) *models.LoginResponse {
	return &models.LoginResponse{
		ErrCode:    0,
		ErrMsg:     "success",
		SessionKey: uuid.New().String(),
		Openid:     strconv.FormatInt(utils.FlakeId(), 10),
		Unionid:    uuid.New().String(),
	}
}
