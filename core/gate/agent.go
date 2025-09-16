package gate

import (
	"net"
)

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
