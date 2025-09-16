package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BaseHandler 基础处理器
type BaseHandler struct{}

// NewBaseHandler 创建基础处理器
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{}
}

// Success 成功响应
func (h *BaseHandler) Success(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}

// Error 错误响应
func (h *BaseHandler) Error(c *gin.Context, statusCode int, message string, err error) {
	response := gin.H{
		"success": false,
		"message": message,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	c.JSON(statusCode, response)
}

// Created 创建成功响应
func (h *BaseHandler) Created(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": message,
		"data":    data,
	})
}
