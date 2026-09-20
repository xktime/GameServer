package http

import (
	"context"
	"errors"
	"fmt"
	"gameserver/conf"
	"gameserver/http/handlers"
	"gameserver/http/routes"
	"gameserver/http/services"
	"log"
	"net"
	stdhttp "net/http"

	"github.com/gin-gonic/gin"
)

// Server HTTP服务器
type Server struct {
	router   *gin.Engine
	server   *stdhttp.Server
	serveErr chan error
}

// NewServer 创建新的HTTP服务器。
func NewServer(addr string) *Server {
	if conf.Server.Debug.Enabled {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	server := newServer(addr, router, router)

	// 设置路由
	server.setupRoutes()
	return server
}

// NewHandlerServer 创建使用指定HTTP Handler的服务器。
func NewHandlerServer(addr string, handler stdhttp.Handler) *Server {
	return newServer(addr, handler, nil)
}

func newServer(addr string, handler stdhttp.Handler, router *gin.Engine) *Server {
	return &Server{
		router:   router,
		server:   &stdhttp.Server{Addr: addr, Handler: handler},
		serveErr: make(chan error, 1),
	}
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
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("http: listen on %s: %w", s.server.Addr, err)
	}
	log.Printf("HTTP服务器启动在: %s", listener.Addr())

	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, stdhttp.ErrServerClosed) {
			err = nil
		}
		s.serveErr <- err
	}()
	return nil
}

// Shutdown 停止接收新请求并等待已启动的服务器退出。
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownErr := s.server.Shutdown(ctx)
	var closeErr error
	if shutdownErr != nil {
		closeErr = s.server.Close()
	}
	serveErr := <-s.serveErr
	return errors.Join(shutdownErr, closeErr, serveErr)
}
