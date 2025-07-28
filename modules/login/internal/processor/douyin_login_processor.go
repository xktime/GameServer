package processor

import (
	"context"
	"encoding/json"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/conf"
	"gameserver/modules/login/internal/models"
)

// todollw 走文件配置
const (
	domain           = "developer.toutiao.com"
	sandBoxDomain    = "open-sandbox.douyin.com"
	code2SessionPath = "/api/apps/v2/jscode2session"
)

type DouyinLoginProcessor struct {
}

func NewDouyinLoginProcessor() *DouyinLoginProcessor {
	return &DouyinLoginProcessor{}
}

func (p *DouyinLoginProcessor) ReqLogin(context context.Context, req *message.C2S_Login) *models.LoginResponse {
	code2SessionReq := &models.Code2SessionRequest{
		AppId:  conf.Server.DouYinInfo.Appid,
		Secret: conf.Server.DouYinInfo.Secret,
		Code:   req.Code,
		ACode:  "",
	}

	code2SessionResp, err := code2Session(context, code2SessionReq)

	if err != nil {
		response := &models.LoginResponse{
			ErrCode: -1,
			ErrMsg:  err.Error(),
		}
		return response
	}

	if code2SessionResp.ErrNo != 0 {
		response := &models.LoginResponse{
			ErrCode: -1,
			ErrMsg:  code2SessionResp.ErrTips,
		}
		return response
	}

	response := &models.LoginResponse{
		ErrCode:    0,
		ErrMsg:     "success",
		SessionKey: code2SessionResp.Data.SessionKey,
		Openid:     code2SessionResp.Data.Openid,
		Unionid:    code2SessionResp.Data.UnionId,
	}
	return response
}

func code2Session(ctx context.Context, req *models.Code2SessionRequest) (*models.Code2SessionResponse, error) {
	reqBodyByte, _ := json.Marshal(req)
	reqDomain := domain
	if conf.Server.DouYinInfo.IsSandBox == 1 {
		reqDomain = sandBoxDomain
	}
	respBody, err := utils.HttpDo(ctx, code2SessionPath, utils.HttpPostMethod, string(reqBodyByte), "https", reqDomain)
	resp := &models.Code2SessionResponse{}
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(respBody), resp)

	return resp, err
}
