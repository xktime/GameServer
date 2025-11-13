package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/conf"
	"gameserver/core/gate"
	"gameserver/modules/login/internal/models"
)

const (
	weChatDomain           = "api.weixin.qq.com"
	weChatCode2SessionPath = "/sns/jscode2session"
)

type WeChatLoginProcessor struct {
}

func NewWechatLoginProcessor() *WeChatLoginProcessor {
	return &WeChatLoginProcessor{}
}

func (p *WeChatLoginProcessor) ReqLogin(agent gate.Agent, ctx context.Context, req *message.C2S_Login) *models.LoginResponse {
	// 构建微信code2session请求
	code2SessionReq := &models.WeChatCode2SessionRequest{
		AppId:  conf.Server.WeChatInfo.Appid,
		Secret: conf.Server.WeChatInfo.Secret,
		JsCode: req.Code,
	}

	// 调用微信API获取用户信息
	code2SessionResp, err := weChatCode2Session(ctx, code2SessionReq)
	if err != nil {
		return &models.LoginResponse{
			ErrCode: -1,
			ErrMsg:  fmt.Sprintf("微信API调用失败: %v", err),
		}
	}

	// 检查微信返回的错误码
	if code2SessionResp.ErrCode != 0 {
		return &models.LoginResponse{
			ErrCode: -1,
			ErrMsg:  fmt.Sprintf("微信登录失败: %s", code2SessionResp.ErrMsg),
		}
	}

	// 返回登录成功响应
	return &models.LoginResponse{
		ErrCode:    0,
		ErrMsg:     "success",
		SessionKey: code2SessionResp.SessionKey,
		Openid:     code2SessionResp.OpenId,
		Unionid:    code2SessionResp.UnionId,
	}
}

// weChatCode2Session 调用微信code2session接口
func weChatCode2Session(ctx context.Context, req *models.WeChatCode2SessionRequest) (*models.WeChatCode2SessionResponse, error) {
	// 构建请求URL参数
	urlParams := fmt.Sprintf("?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		req.AppId, req.Secret, req.JsCode)

	// 发送GET请求到微信API
	respBody, err := utils.HttpGet(ctx, weChatCode2SessionPath+urlParams, "", "https", weChatDomain)
	if err != nil {
		return nil, err
	}

	// 解析响应
	resp := &models.WeChatCode2SessionResponse{}
	err = json.Unmarshal([]byte(respBody), resp)
	if err != nil {
		return nil, fmt.Errorf("解析微信响应失败: %v", err)
	}

	return resp, nil
}
