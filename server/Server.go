package main

import (
	"fmt"
	"net"
)

// iServer 接口的实现 定义一个Server的服务器模块
type Server struct {
	// 服务器的名称
	Name string
	//  服务器版本
	IPVersion string
	//服务器监听的ip
	IP string
	//服务器监听的端口
	Port int
}

// 启动服务器
func (s *Server) Start() {
	fmt.Printf("[Start] Server Listenner at IP :%s, Port %d, is starting \n", s.IP, s.Port)

	go func() {
		// 获取一个TCP的addr
		addr, err := net.ResolveTCPAddr(s.IPVersion, fmt.Sprintf("%s:%d", s.IP, s.Port))
		if err != nil {
			fmt.Println("resolve tcp addr err :", err)
			return
		}
		// 监听服务器的地质
		listenner, err := net.ListenTCP(s.IPVersion, addr)
		if err != nil {
			fmt.Println("listen ", s.IPVersion, err)
			return
		}
		fmt.Println("start Zinx server succ,", s.Name, "succ, Listenning ..")
		// 阻塞等待客户端连接
		// 处理客户端连接业务
		for {
			// 如果有客户端连接 返回
			conn, err := listenner.AcceptTCP()
			if err != nil {
				fmt.Println("Accept err :", err)
				return
			}

			// 客户端已经建立连接， 做一些业务处理
			// 做一个最大512字节的回显业务
			go func() {
				for {
					buf := make([]byte, 512)
					cnt, err := conn.Read(buf)
					if err != nil {
						fmt.Println("recv buf err :", err)
						continue
					}
					if _, err := conn.Write(buf[:cnt]); err != nil {
						fmt.Println("write back bur err", err)
						continue
					}
				}
			}()
		}
	}()
}

// 停止服务器
func (s *Server) Stop() {
	// 将服务器开辟的资源，连接停止 释放
}

// 运行服务器
func (s *Server) Serve() {
	s.Start()

	//TODO  做启动服务后的额外业务
	// 阻塞状态
	select {}
}

// 初始化服务器的方法
func NewServer(name string) *Server {
	s := &Server{
		Name:      name,
		IPVersion: "tcp4",
		IP:        "0.0.0.0",
		Port:      8999,
	}
	return s
}
