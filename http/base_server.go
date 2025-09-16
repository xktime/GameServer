package http

import (
	"fmt"
	"gameserver/conf"
	"gameserver/http/handlers"
	"gameserver/http/routes"
	"gameserver/http/services"
	"log"

	"github.com/gin-gonic/gin"
)

// Server HTTP服务器
type Server struct {
	router *gin.Engine
}

// NewServer 创建新的HTTP服务器
func Start() {
	if conf.Server.Debug.Enabled {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	server := &Server{
		router: gin.New(),
	}

	// 设置路由
	server.setupRoutes()

	go server.Start()
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	// 创建处理器
	gmHandler := handlers.NewGmHandler(services.NewGmService())

	// 设置基础路由（包含中间件）
	routes.SetupBaseRoutes(s.router, gmHandler)
}

// Start 启动服务器
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", "0.0.0.0", conf.Server.HttpPort)
	log.Printf("HTTP服务器启动在: %s", addr)

	return s.router.Run(addr)
}
