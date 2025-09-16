package handlers

import (
	"gameserver/http/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
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

// http://127.0.0.1:8080/api/v1/gm/addItem?playerId=1&itemId=2&count=3
func (h *GmHandler) AddItem(c *gin.Context) {
	playerId := c.Query("playerId")
	itemId := c.Query("itemId")
	count := c.Query("count")

	// 将字符串参数转换为对应的整型类型
	playerIdInt64, err1 := strconv.ParseInt(playerId, 10, 64)
	itemIdInt32, err2 := strconv.ParseInt(itemId, 10, 32)
	countInt32, err3 := strconv.ParseInt(count, 10, 32)
	if err1 != nil || err2 != nil || err3 != nil {
		h.Error(c, http.StatusBadRequest, "参数格式错误", nil)
		return
	}
	result := h.gmService.AddItem(playerIdInt64, int32(itemIdInt32), int32(countInt32))
	if result == services.ResultPlayerOffline {
		h.Error(c, http.StatusOK, "玩家不在线", nil)
		return
	}

	h.Success(c, gin.H{
		"playerId": playerId,
		"itemId":   itemId,
		"count":    count,
	}, "发送成功")
}
