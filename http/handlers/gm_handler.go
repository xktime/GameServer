package handlers

import (
	"gameserver/http/services"
)

// GmHandler GM处理器
type GmHandler struct {
	*BaseHandler
	gmService *services.GmService
}

// NewGmHandler 创建健康检查处理器
func NewGmHandler(gmService *services.GmService) *GmHandler {
	return &GmHandler{
		BaseHandler: NewBaseHandler(),
		gmService:   gmService,
	}
}
