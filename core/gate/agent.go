package gate

import (
	"net"
)

type ConnectionID uint64

type ConnectionAgent interface {
	Agent
	ConnectionID() ConnectionID
}

type Agent interface {
	WriteMsg(msg interface{})
	WriteMsgWithSeq(msg interface{}, seq uint32)
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Close()
	Destroy()
	UserData() interface{}
	SetUserData(data interface{})
}
