package gate

import (
	"gameserver/core/chanrpc"
	"gameserver/core/log"
	"gameserver/core/network"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type Gate struct {
	Processor    network.Processor
	AgentChanRPC *chanrpc.Server
	// OnConnectionClosing must only invalidate local state; it runs before any
	// transport close or asynchronous CloseAgent notification.
	OnConnectionClosing func(ConnectionID)
	// WebSocket 连接配置
	WSAddr   string
	CertFile string
	KeyFile  string
	// TCP 连接配置
	TCPAddr          string
	MaxConnNum       int
	PendingWriteNum  int
	HTTPTimeout      time.Duration
	LenMsgLen        int
	MaxMsgLen        uint32
	LittleEndian     bool
	nextConnectionID atomic.Uint64
}

func (gate *Gate) Run(closeSig chan bool) {
	var wsServer *network.WSServer
	if gate.WSAddr != "" {
		wsServer = new(network.WSServer)
		wsServer.Addr = gate.WSAddr
		wsServer.MaxConnNum = gate.MaxConnNum
		wsServer.PendingWriteNum = gate.PendingWriteNum
		wsServer.MaxMsgLen = gate.MaxMsgLen
		wsServer.HTTPTimeout = gate.HTTPTimeout
		wsServer.CertFile = gate.CertFile
		wsServer.KeyFile = gate.KeyFile
		wsServer.NewAgent = func(conn *network.WSConn) network.Agent {
			a := gate.newAgent(conn)
			gate.registerAgent(a)
			return a
		}
	}

	var tcpServer *network.TCPServer
	if gate.TCPAddr != "" {
		tcpServer = new(network.TCPServer)
		tcpServer.Addr = gate.TCPAddr
		tcpServer.MaxConnNum = gate.MaxConnNum
		tcpServer.PendingWriteNum = gate.PendingWriteNum
		tcpServer.LenMsgLen = gate.LenMsgLen
		tcpServer.MaxMsgLen = gate.MaxMsgLen
		tcpServer.LittleEndian = gate.LittleEndian
		tcpServer.NewAgent = func(conn *network.TCPConn) network.Agent {
			a := gate.newAgent(conn)
			gate.registerAgent(a)
			return a
		}
	}

	if wsServer != nil {
		wsServer.Start()
	}
	if tcpServer != nil {
		tcpServer.Start()
	}
	<-closeSig
	if wsServer != nil {
		wsServer.Close()
	}
	if tcpServer != nil {
		tcpServer.Close()
	}
}

func (gate *Gate) registerAgent(a *agent) {
	if gate.AgentChanRPC == nil {
		return
	}
	if err := gate.AgentChanRPC.Call0("NewAgent", a); err != nil {
		log.Error("登记新连接失败: %v", err)
	}
}

func (gate *Gate) OnDestroy() {}

type agent struct {
	connectionID  ConnectionID
	conn          network.Conn
	gate          *Gate
	userDataMutex sync.RWMutex
	userData      interface{}
	closeData     interface{}
	closing       sync.Once
}

func (gate *Gate) newAgent(conn network.Conn) *agent {
	return &agent{
		connectionID: ConnectionID(gate.nextConnectionID.Add(1)),
		conn:         conn,
		gate:         gate,
	}
}

func (a *agent) Run() {
	defer a.invalidateConnection()
	for {
		data, err := a.conn.ReadMsg()
		if err != nil {
			log.Debug("read message: %v", err)
			break
		}

		if a.gate.Processor != nil {
			msg, err := a.gate.Processor.Unmarshal(data)
			if err != nil {
				log.Debug("unmarshal message error: %v", err)
				break
			}
			err = a.gate.Processor.Route(msg, a)
			if err != nil {
				log.Debug("route message error: %v", err)
				break
			}
		}
	}
}

func (a *agent) OnClose() {
	a.invalidateConnection()
	if a.gate.AgentChanRPC != nil {
		err := a.gate.AgentChanRPC.Call0("CloseAgent", a, a.closeSnapshot())
		if err != nil {
			log.Error("chanrpc error: %v", err)
		}
	}
}

func (a *agent) WriteMsg(msg interface{}) {
	if a.gate.Processor != nil {
		data, err := a.gate.Processor.Marshal(msg, 0)
		if err != nil {
			log.Error("marshal message %v error: %v", reflect.TypeOf(msg), err)
			return
		}
		err = a.conn.WriteMsg(data...)
		if err != nil {
			log.Error("write message %v error: %v", reflect.TypeOf(msg), err)
		}
	}
}

func (a *agent) WriteMsgWithSeq(msg interface{}, seq uint32) {
	if a.gate.Processor != nil {
		data, err := a.gate.Processor.Marshal(msg, seq)
		if err != nil {
			log.Error("marshal message %v error: %v", reflect.TypeOf(msg), err)
			return
		}
		err = a.conn.WriteMsg(data...)
		if err != nil {
			log.Error("write message %v error: %v", reflect.TypeOf(msg), err)
		}
	}
}

func (a *agent) LocalAddr() net.Addr {
	return a.conn.LocalAddr()
}

func (a *agent) RemoteAddr() net.Addr {
	return a.conn.RemoteAddr()
}

func (a *agent) Close() {
	a.invalidateConnection()
	a.conn.Close()
}

func (a *agent) Destroy() {
	a.invalidateConnection()
	a.conn.Destroy()
}

func (a *agent) invalidateConnection() {
	a.closing.Do(func() {
		if a.gate != nil && a.gate.OnConnectionClosing != nil {
			a.gate.OnConnectionClosing(a.connectionID)
		}
	})
}

func (a *agent) ConnectionID() ConnectionID {
	return a.connectionID
}

func (a *agent) UserData() interface{} {
	a.userDataMutex.RLock()
	defer a.userDataMutex.RUnlock()
	return a.userData
}

func (a *agent) SetUserData(data interface{}) {
	a.userDataMutex.Lock()
	defer a.userDataMutex.Unlock()
	if data != nil {
		a.closeData = data
	}
	a.userData = data
}

func (a *agent) closeSnapshot() interface{} {
	a.userDataMutex.RLock()
	defer a.userDataMutex.RUnlock()
	return a.closeData
}
