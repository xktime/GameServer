package gate

import (
	"errors"
	"net"
	"testing"
)

type closingConn struct{ beforeClose func() }

func (*closingConn) ReadMsg() ([]byte, error) { return nil, errors.New("peer closed") }
func (*closingConn) WriteMsg(...[]byte) error { return nil }
func (*closingConn) LocalAddr() net.Addr      { return nil }
func (*closingConn) RemoteAddr() net.Addr     { return nil }
func (c *closingConn) Close()                 { c.beforeClose() }
func (c *closingConn) Destroy()               { c.beforeClose() }

func TestGateInvalidatesConnectionBeforeClosingAndOnlyOnce(t *testing.T) {
	for _, first := range []string{"close", "destroy", "read", "notification"} {
		t.Run(first, func(t *testing.T) {
			invalidations := 0
			server := &Gate{OnConnectionClosing: func(id ConnectionID) {
				if id != 1 {
					t.Fatalf("connection = %d", id)
				}
				invalidations++
			}}
			conn := &closingConn{beforeClose: func() {
				if invalidations != 1 {
					t.Fatal("transport closed before invalidation")
				}
			}}
			agent := server.newAgent(conn)
			switch first {
			case "close":
				agent.Close()
			case "destroy":
				agent.Destroy()
			case "read":
				agent.Run()
			case "notification":
				agent.OnClose()
			}
			agent.Close()
			agent.Destroy()
			agent.OnClose()
			if invalidations != 1 {
				t.Fatalf("invalidations = %d", invalidations)
			}
		})
	}
}
