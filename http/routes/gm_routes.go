package routes

import (
	"gameserver/http/handlers"
	"gameserver/http/middleware"

	"github.com/gin-gonic/gin"
)

// SetupBaseRoutes 设置基础路由
func SetupBaseRoutes(router *gin.Engine, gmHandler *handlers.GmHandler) {
	// 应用中间件
	router.Use(middleware.Logger())
	router.Use(middleware.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RequestSizeLimiterDefault())

	// API路由组
	// api := router.Group("/api/v1")
	// {
	// 	// CDK相关路由
	// 	gm := api.Group("/gm")
	// 	{

	// 	}
	// }

	// 根路径
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "欢迎使用GM Server",
			"version": "1.0.0",
			"docs":    "/api/v1",
		})
	})
}
