package processor

import (
	"context"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/modules/login/internal/models"
	"math/rand"
	"strconv"

	"github.com/google/uuid"
)

type WeChatLoginProcessor struct {
}

func NewWechatLoginProcessor() *WeChatLoginProcessor {
	return &WeChatLoginProcessor{
	}
}

var cache = make([]string, 0)
func (p *WeChatLoginProcessor) ReqLogin(context context.Context, req *message.C2S_Login) *models.LoginResponse {
		cache = append(cache, strconv.FormatInt(utils.FlakeId(), 10))
		openId := strconv.FormatInt(utils.FlakeId(), 10)
		if rand.Intn(2) == 1 {
			if len(cache) > 0 {
				openId = cache[rand.Intn(len(cache))]
			}
		} else {
			newOpenId := strconv.FormatInt(utils.FlakeId(), 10)
			cache = append(cache, newOpenId)
			openId = newOpenId
		}

		return &models.LoginResponse{
			ErrCode:    0,
			ErrMsg:     "success",
			SessionKey: uuid.New().String(),
			Openid:     openId,
			Unionid:    uuid.New().String(),
		}
	}
