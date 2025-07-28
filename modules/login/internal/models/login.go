package models

import "google.golang.org/protobuf/runtime/protoimpl"

type LoginRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Code string `protobuf:"bytes,1,opt,name=code,proto3" json:"code,omitempty"` // 用户登录code
}

// LoginResponse 用于表示登录接口的返回结果。
// 字段说明：
// - ErrCode：接口返回码，通常为0表示成功，非0表示失败或异常。
// - ErrMsg：接口返回信息，描述错误或成功的详细信息。
// - Token：用户登录成功后分配的令牌（token），用于后续鉴权和会话保持。
// - Openid：用户在第三方平台（如抖音）下的唯一标识，用于标识当前用户身份。
// - Unionid：用户在第三方平台下的全局唯一标识（同一用户在不同应用下的唯一ID），用于多应用间用户身份的统一。
type LoginResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ErrCode    int32  `protobuf:"varint,1,opt,name=errCode,proto3" json:"errCode,omitempty"`
	ErrMsg     string `protobuf:"bytes,2,opt,name=errMsg,proto3" json:"errMsg,omitempty"`
	SessionKey string `protobuf:"bytes,3,opt,name=sessionKey,proto3" json:"sessionKey,omitempty"`
	Openid     string `protobuf:"bytes,4,opt,name=openid,proto3" json:"openid,omitempty"`
	Unionid    string `protobuf:"bytes,5,opt,name=unionid,proto3" json:"unionid,omitempty"`
}

// 抖音登录
type Code2SessionRequest struct {
	AppId  string `json:"appid"`
	Secret string `json:"secret"`
	Code   string `json:"code"`
	ACode  string `json:"anonymous_code"`
}

type Code2SessionResponse struct {
	ErrNo   int64  `json:"err_no"`
	ErrTips string `json:"err_tips"`

	Data Code2SessionResponseData `json:"data"`
}

type Code2SessionResponseData struct {
	SessionKey string `json:"session_key"`
	Openid     string `json:"openid"`
	AOpenId    string `json:"anonymous_openid"`
	UnionId    string `json:"unionid"`
	DOpenid    string `json:"dopenid"`
}
