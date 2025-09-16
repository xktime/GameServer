package processor

import (
	"context"
	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
	"net"
	"strconv"

	"github.com/google/uuid"
)

type AccountCache struct {
	Code   string `bson:"_id"`
	OpenId string `bson:"openId"`
}

func (a AccountCache) GetPersistId() interface{} {
	return a.Code
}

type DefaultLoginProcessor struct {
}

func NewDefaultLoginProcessor() *DefaultLoginProcessor {
	return &DefaultLoginProcessor{}
}

func (p *DefaultLoginProcessor) ReqLogin(agent gate.Agent, context context.Context, req *message.C2S_Login) *models.LoginResponse {
	ip := agent.RemoteAddr().(*net.TCPAddr).IP.String()
	code := ip + "_" + req.Code
	accountCache, err := mongodb.FindOneById[AccountCache](code)
	if err != nil {
		return &models.LoginResponse{
			ErrCode: -1,
			ErrMsg:  err.Error(),
		}
	}
	if accountCache != nil {
		return &models.LoginResponse{
			ErrCode:    0,
			ErrMsg:     "success",
			SessionKey: uuid.New().String(),
			Openid:     accountCache.OpenId,
			Unionid:    uuid.New().String(),
		}
	} else {
		openId := strconv.FormatInt(utils.FlakeId(), 10)
		mongodb.Save(AccountCache{
			Code:   code,
			OpenId: openId,
		})
		return &models.LoginResponse{
			ErrCode:    0,
			ErrMsg:     "success",
			SessionKey: uuid.New().String(),
			Openid:     openId,
			Unionid:    uuid.New().String(),
		}
	}
}
