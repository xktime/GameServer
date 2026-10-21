package managers

import (
	"context"
	"gameserver/common/base/actor"
	"gameserver/common/session"
	"gameserver/core/gate"
	"gameserver/modules/game/playersession"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testConnectionAgent struct {
	id       gate.ConnectionID
	address  net.Addr
	closed   atomic.Bool
	dataMu   sync.RWMutex
	userData any
}

var _ gate.ConnectionAgent = (*testConnectionAgent)(nil)

func (*testConnectionAgent) WriteMsg(any)                {}
func (*testConnectionAgent) WriteMsgWithSeq(any, uint32) {}
func (a *testConnectionAgent) LocalAddr() net.Addr       { return a.address }
func (a *testConnectionAgent) RemoteAddr() net.Addr      { return a.address }
func (a *testConnectionAgent) ConnectionID() gate.ConnectionID {
	return a.id
}
func (a *testConnectionAgent) Close() { a.closed.Store(true) }
func (*testConnectionAgent) Destroy() {}
func (a *testConnectionAgent) UserData() any {
	a.dataMu.RLock()
	defer a.dataMu.RUnlock()
	return a.userData
}
func (a *testConnectionAgent) SetUserData(data any) {
	a.dataMu.Lock()
	defer a.dataMu.Unlock()
	a.userData = data
}

func newConnectManagerTestHarness(t *testing.T, now func() time.Time) *ConnectManager {
	t.Helper()
	system := actor.NewActorSystem(time.Second)
	t.Cleanup(func() {
		if err := system.Stop(context.Background()); err != nil {
			t.Errorf("停止 ActorSystem: %v", err)
		}
	})
	scope, err := system.NewScope("connect-manager-test")
	if err != nil {
		t.Fatalf("创建 Scope: %v", err)
	}
	manager, err := newConnectManager(context.Background(), scope, now, playersession.NewState())
	if err != nil {
		t.Fatalf("创建 ConnectManager: %v", err)
	}
	return manager
}

func TestConnectManagerTracksConnectionsByConnectionID(t *testing.T) {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	manager := newConnectManagerTestHarness(t, func() time.Time { return now })
	sharedAddress := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9000}
	first := &testConnectionAgent{id: 1, address: sharedAddress}
	second := &testConnectionAgent{id: 2, address: sharedAddress}

	manager.Register(first)
	manager.Register(second)
	if got := manager.GetActiveClients(); got != 2 {
		t.Fatalf("活跃连接数 = %d，期望 2", got)
	}

	manager.RemoveClient(first.ConnectionID())
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("移除首个 ConnectionID 后活跃连接数 = %d，期望 1", got)
	}
}

func TestUnauthenticatedHeartbeatDoesNotExtendLoginDeadline(t *testing.T) {
	connectedAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	now := connectedAt
	manager := newConnectManagerTestHarness(t, func() time.Time { return now })
	agent := &testConnectionAgent{id: 1, address: &net.TCPAddr{Port: 9000}}

	manager.Register(agent)
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("登记后的活跃连接数 = %d，期望 1", got)
	}

	now = connectedAt.Add(LoginTimeout - time.Second)
	manager.UpdateHeartbeat(agent.ConnectionID())
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("心跳后的活跃连接数 = %d，期望 1", got)
	}

	now = connectedAt.Add(LoginTimeout + time.Nanosecond)
	manager.OnTimer()
	if got := manager.GetActiveClients(); got != 0 {
		t.Fatalf("超过登录截止时间后的活跃连接数 = %d，期望 0", got)
	}
	if !agent.closed.Load() {
		t.Fatal("超过登录截止时间的未认证连接未关闭")
	}
}

func TestAuthenticatedConnectionUsesHeartbeatDeadline(t *testing.T) {
	connectedAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	now := connectedAt
	manager := newConnectManagerTestHarness(t, func() time.Time { return now })
	agent := &testConnectionAgent{id: 1, address: &net.TCPAddr{Port: 9000}}

	manager.Register(agent)
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("登记后的活跃连接数 = %d，期望 1", got)
	}

	now = connectedAt.Add(20 * time.Second)
	agent.SetUserData(session.PlayerSession{ID: 1, AccountID: "account-1", PlayerID: 1001})
	manager.UpdateHeartbeat(agent.ConnectionID())
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("认证后的活跃连接数 = %d，期望 1", got)
	}

	now = connectedAt.Add(20*time.Second + HeartbeatTimeout)
	manager.OnTimer()
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("到达心跳截止时间时活跃连接数 = %d，期望 1", got)
	}
	if agent.closed.Load() {
		t.Fatal("刚到心跳截止时间的已认证连接不应提前关闭")
	}

	now = now.Add(time.Nanosecond)
	manager.OnTimer()
	if got := manager.GetActiveClients(); got != 0 {
		t.Fatalf("超过心跳截止时间后的活跃连接数 = %d，期望 0", got)
	}
	if !agent.closed.Load() {
		t.Fatal("超过心跳截止时间的已认证连接未关闭")
	}
}

func TestConnectManagerRecordsEventTimeBeforeActorQueueDelay(t *testing.T) {
	connectedAt := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	now := connectedAt
	manager := newConnectManagerTestHarness(t, func() time.Time { return now })
	agent := &testConnectionAgent{id: 1, address: &net.TCPAddr{Port: 9000}}

	releaseRegister := blockConnectManager(t, manager)
	manager.Register(agent)
	now = connectedAt.Add(5 * time.Second)
	releaseRegister()
	clients := manager.GetAllClients()
	if got := clients[agent.ConnectionID()].ConnectedAt; got != connectedAt {
		t.Fatalf("连接登记时间 = %v，期望事件收到时间 %v", got, connectedAt)
	}

	heartbeatAt := connectedAt.Add(20 * time.Second)
	now = heartbeatAt
	releaseHeartbeat := blockConnectManager(t, manager)
	manager.UpdateHeartbeat(agent.ConnectionID())
	now = heartbeatAt.Add(5 * time.Second)
	releaseHeartbeat()
	clients = manager.GetAllClients()
	if got := clients[agent.ConnectionID()].LastHeartbeat; got != heartbeatAt {
		t.Fatalf("心跳时间 = %v，期望事件收到时间 %v", got, heartbeatAt)
	}
}

func TestConnectManagerRejectsStaleLoginAttempts(t *testing.T) {
	manager := newConnectManagerTestHarness(t, time.Now)
	agent := &testConnectionAgent{id: 1, address: &net.TCPAddr{Port: 9000}}
	manager.Register(agent)
	if got := manager.GetActiveClients(); got != 1 {
		t.Fatalf("登记后的活跃连接数 = %d，期望 1", got)
	}

	first, err := manager.BeginLogin(agent.ConnectionID(), 1)
	if err != nil {
		t.Fatalf("开始首个登录请求: %v", err)
	}
	second, err := manager.BeginLogin(agent.ConnectionID(), 2)
	if err != nil {
		t.Fatalf("开始第二个登录请求: %v", err)
	}
	if first.IsCurrent("") {
		t.Fatal("旧登录请求仍被识别为当前请求")
	}
	if !second.IsCurrent("") {
		t.Fatal("最新登录请求未被识别为当前请求")
	}
	release := blockConnectManager(t, manager)
	manager.RemoveClient(agent.ConnectionID())
	if second.IsCurrent("") {
		t.Fatal("连接移除后仍接受旧登录响应")
	}
	release()
}

func blockConnectManager(t *testing.T, manager *ConnectManager) func() {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	if err := actor.Tell(context.Background(), manager.Ref(), func(actor.Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("阻塞 ConnectManager 队列: %v", err)
	}
	<-started
	return func() { close(release) }
}
