package routers

import (
	"GameServer/server/znet/messages"
	"GameServer/server/znet/routers/ClientToServer"
	"GameServer/server/znet/routers/ServerToClient"
	"github.com/aceld/zinx/ziface"
	"log"
	"sync"
)

type Routers struct {
	s2CMessages []messages.Message
	c2SMessages []messages.Message
}

var (
	mu       sync.Mutex
	instance *Routers
)

func GetInstance() *Routers {
	if instance == nil {
		mu.Lock()
		defer mu.Unlock()
		if instance == nil {
			instance = &Routers{}
			instance.Load()
		}
	}
	return instance
}

// Load todo 能不能反射绑定?
func (r *Routers) Load() {
	// 加载S2CMessages
	r.s2CMessages = append(r.s2CMessages, &ServerToClient.S2CLogin{})

	// 加载C2SMessages
	r.c2SMessages = append(r.c2SMessages, &ClientToServer.C2SLogin{})
}

func (r *Routers) GetS2CMessages() []messages.Message {
	return r.s2CMessages
}

func (r *Routers) GetC2SMessages() []messages.Message {
	return r.c2SMessages
}

func (r *Routers) RegisterC2SRouters(server ziface.IServer) {
	registerMessages := r.GetC2SMessages()
	for _, message := range registerMessages {
		switch message.(type) {
		case ziface.IRouter:
			server.AddRouter(message.GetMessageId(), message.(ziface.IRouter))
		default:
			log.Fatal(message.GetMessageId(), "未实现IRouter")
		}
	}
}

func (r *Routers) RegisterS2CRouters(client ziface.IClient) {
	registerMessages := r.GetS2CMessages()
	for _, message := range registerMessages {
		switch message.(type) {
		case ziface.IRouter:
			client.AddRouter(message.GetMessageId(), message.(ziface.IRouter))
		default:
			log.Fatal(message.GetMessageId(), "未实现IRouter")
		}
	}
}
