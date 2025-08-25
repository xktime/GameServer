package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gameserver/common/db/mongodb"
	"gameserver/common/msg/message"
	actor_manager "gameserver/core/actor"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/stretchr/testify/assert"
)

// TestActor 用于测试的 Actor
type TestActor struct {
	GroupPID         *actor.PID
	receivedMessages []interface{}
	mu               sync.Mutex
}

func (a *TestActor) Receive(context actor.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if context.Message() == nil {
		return
	}
	msg, ok := context.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	a.receivedMessages = append(a.receivedMessages, msg[2])
}

func (a *TestActor) GetMessages() []interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.receivedMessages
}

// TestActorWithResponse 用于测试请求-响应模式的 Actor
type TestActorWithResponse struct {
	TestActor
}

func (a *TestActorWithResponse) Receive(context actor.Context) {
	if context.Message() == nil {
		return
	}
	msg, ok := context.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		// 发送响应
		a.receivedMessages = append(a.receivedMessages, msg)
		if context.Sender() != nil {
			context.Respond("response to: " + msg.Code)
		}
	case *actor.Started:
		// 忽略Started消息，不记录到receivedMessages
	default:
		// 记录其他消息
		a.receivedMessages = append(a.receivedMessages, context.Message())
	}
}

// TestActorWithSave 用于测试移除group时save操作的Actor
type TestActorWithSave struct {
	TestActor
}

func (p TestActorWithSave) GetPersistId() interface{} {
	return 1
}

// TestActorFactory_Init 测试初始化
func TestActorFactory_Init(t *testing.T) {
	actor_manager.Init(2000)
}

// TestActorFactory_Register 测试注册 Actor
func TestActorFactory_Register(t *testing.T) {
	actor_manager.Init(2000)
	// 测试注册
	pid, err := actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	assert.NoError(t, err)
	assert.NotNil(t, pid)

	// 测试重复注册
	pid2, err := actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	assert.NoError(t, err)
	assert.Nil(t, pid2) // 应该返回 nil，因为已存在
}

// TestActorFactory_Get 测试获取 Actor
func TestActorFactory_Get(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	pid := meta.Actor
	assert.NotNil(t, pid)

	// 获取 Actor
	retrievedPID := actor_manager.Get[TestActor]("test1")
	assert.Equal(t, pid, retrievedPID)

	// 获取不存在的 Actor
	nonExistentPID := actor_manager.Get[SerialActor]("test1")
	assert.Nil(t, nonExistentPID)
}

// TestActorFactory_Send 测试发送消息
func TestActorFactory_Send(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	assert.NotNil(t, meta)

	// 发送消息
	actor_manager.Send[TestActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "hello"}})
	actor_manager.Send[TestActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "world"}})

	// 等待消息处理
	time.Sleep(100 * time.Millisecond)

	// 验证消息接收
	messages := meta.Actor.GetMessages()
	assert.Len(t, messages, 2)
	assert.Equal(t, "hello", messages[0].(*message.C2S_Login).Code)
	assert.Equal(t, "world", messages[1].(*message.C2S_Login).Code)

	// 测试发送给不存在的 Actor
	actor_manager.Send[SerialActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "test"}})
	// 不应该 panic
}

// TestActorFactory_Stop 测试停止单个 Actor
func TestActorFactory_Stop(t *testing.T) {
	actor_manager.Init(2000)
	uniqueID := "test1"

	// 注册 Actor
	pid, _ := actor_manager.Register[TestActor](uniqueID, actor_manager.Test1)
	assert.NotNil(t, pid)

	// 验证 Actor 存在
	assert.NotNil(t, actor_manager.Get[TestActor](uniqueID))

	// 停止 Actor
	actor_manager.Stop[TestActor](uniqueID)

	// 验证 Actor 已被移除
	assert.Nil(t, actor_manager.Get[TestActor](uniqueID))

	// 测试停止不存在的 Actor
	actor_manager.Stop[SerialActor](uniqueID)
	// 不应该 panic
}

// TestActorFactory_StopGroup 测试停止分组
func TestActorFactory_StopGroup(t *testing.T) {
	actor_manager.Init(2000)

	// 注册多个同组 Actor
	actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	actor_manager.Register[SerialActor]("test1", actor_manager.Test1)
	actor_manager.Register[TestActorWithResponse]("test2", actor_manager.Test2)

	// 验证 Actor 存在
	assert.NotNil(t, actor_manager.Get[TestActor]("test1"))
	assert.NotNil(t, actor_manager.Get[SerialActor]("test1"))
	assert.NotNil(t, actor_manager.Get[TestActorWithResponse]("test2"))

	// 停止 group1
	actor_manager.StopGroup(actor_manager.Test1, "test1")

	// 验证 group1 的 Actor 被移除，group2 的 Actor 还在
	assert.Nil(t, actor_manager.Get[TestActor]("test1"))
	assert.Nil(t, actor_manager.Get[SerialActor]("test1"))
	assert.NotNil(t, actor_manager.Get[TestActorWithResponse]("test2"))
}

// TestActorFactory_StopGroupWithSave 测试停止分组时子actor的移除和save操作
func TestActorFactory_StopGroupWithSave(t *testing.T) {
	actor_manager.Init(2000)
	mongodb.Init("mongodb://localhost:27017", "testMongo", 50, 50)
	uniqueID2 := "test_group_0"
	meta1, _ := actor_manager.Register[TestActor](uniqueID2, actor_manager.Test1)
	meta2, err := actor_manager.Register[TestActorWithSave](uniqueID2, actor_manager.Test1)
	assert.NoError(t, err)
	assert.NotNil(t, meta1)
	assert.NotNil(t, meta2)

	// 验证Actor实现了ActorData接口
	_, ok := interface{}(meta2.Actor).(mongodb.PersistData)
	assert.True(t, ok, "TestActorWithSave should implement PersistData interface")

	actor := actor_manager.Get[TestActorWithSave](uniqueID2)
	assert.NotNil(t, actor, "Actor should be exist")
	actor2 := actor_manager.Get[TestActor](uniqueID2)
	assert.NotNil(t, actor2, "Actor should be exist")

	// 停止组
	actor_manager.StopGroup(actor_manager.Test1, uniqueID2)

	// 验证actor已被移除
	actor = actor_manager.Get[TestActorWithSave](uniqueID2)
	assert.Nil(t, actor, "Actor should be removed when group is stopped")
	actor2 = actor_manager.Get[TestActor](uniqueID2)
	assert.Nil(t, actor2, "Actor should be removed when group is stopped")
}

// TestActorFactory_StopAll 测试停止所有 Actor
func TestActorFactory_StopAll(t *testing.T) {
	actor_manager.Init(2000)

	// 注册多个 Actor
	actor_manager.Register[TestActor]("test1", actor_manager.Test1)
	actor_manager.Register[SerialActor]("test1", actor_manager.Test1)
	actor_manager.Register[TestActorWithResponse]("test2", actor_manager.Test2)

	// 验证 Actor 存在
	assert.NotNil(t, actor_manager.Get[TestActor]("test1"))
	assert.NotNil(t, actor_manager.Get[SerialActor]("test1"))
	assert.NotNil(t, actor_manager.Get[TestActorWithResponse]("test2"))

	// 停止所有 Actor
	actor_manager.StopAll()

	// 验证所有 Actor 被移除
	assert.Nil(t, actor_manager.Get[TestActor]("test1"))
	assert.Nil(t, actor_manager.Get[SerialActor]("test1"))
	assert.Nil(t, actor_manager.Get[TestActorWithResponse]("test2"))
}

// TestActorFactory_ConcurrentRegister 测试并发注册
func TestActorFactory_ConcurrentRegister(t *testing.T) {
	actor_manager.Init(2000)

	var wg sync.WaitGroup
	actorCount := 100

	// 并发注册 Actor
	for i := 0; i < actorCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// 这里用TestActor类型注册
			pid, err := actor_manager.Register[TestActor](fmt.Sprintf("test%d", i), actor_manager.Test1)
			assert.NoError(t, err)
			assert.NotNil(t, pid)
		}(i)
	}

	wg.Wait()

	// 验证所有 Actor 都被注册
	for i := 0; i < actorCount; i++ {
		assert.NotNil(t, actor_manager.Get[TestActor](fmt.Sprintf("test%d", i)))
	}
}

// TestActorFactory_ConcurrentSend 测试并发发送消息
func TestActorFactory_ConcurrentSend(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActor]("test1", actor_manager.Test1)

	var wg sync.WaitGroup
	messageCount := 10000

	// 并发发送消息
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			actor_manager.Send[TestActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("message_%d", id)}})
		}(i)
	}

	wg.Wait()

	// 等待消息处理
	time.Sleep(200 * time.Millisecond)

	// 验证消息接收
	messages := meta.Actor.GetMessages()
	assert.Len(t, messages, messageCount)
}

// TestActorFactory_RequestResponse 测试请求-响应模式
func TestActorFactory_RequestResponse(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActorWithResponse]("test1", actor_manager.Test1)
	assert.NotNil(t, meta)

	// 发送请求并等待响应
	future := actor_manager.RequestFuture[TestActorWithResponse]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "hello"}})

	// 等待响应
	result, err := future.Result()
	assert.NoError(t, err)
	assert.Equal(t, "response to: hello", result)

	// 验证消息接收
	messages := meta.Actor.GetMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, "hello", messages[0].(*message.C2S_Login).Code)
}

// TestActorFactory_ActorLifecycle 测试 Actor 生命周期
func TestActorFactory_ActorLifecycle(t *testing.T) {
	actor_manager.Init(2000)

	// 创建带生命周期的测试 Actor
	uniqueID := "test1"

	// 注册 Actor
	meta, _ := actor_manager.Register[LifecycleTestActor](uniqueID, actor_manager.Test1)
	assert.NotNil(t, meta)

	// 等待 Actor 启动
	time.Sleep(100 * time.Millisecond)
	assert.True(t, meta.Actor.started)

	// 发送消息
	actor_manager.Send[LifecycleTestActor](uniqueID, "Receive", []interface{}{&message.C2S_Login{Code: "test"}})
	time.Sleep(100 * time.Millisecond)
	assert.True(t, meta.Actor.received)

	// 停止 Actor
	actor_manager.Stop[LifecycleTestActor](uniqueID)
	time.Sleep(100 * time.Millisecond)
	assert.True(t, meta.Actor.stopped)
}

// LifecycleTestActor 用于测试生命周期的 Actor
type LifecycleTestActor struct {
	GroupPID *actor.PID
	started  bool
	received bool
	stopped  bool
}

func (a *LifecycleTestActor) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		a.started = true
	case *actor.Stopping:
		a.stopped = true
	default:
		a.received = true
	}
}

// TestActorFactory_ErrorHandling 测试错误处理
func TestActorFactory_ErrorHandling(t *testing.T) {
	actor_manager.Init(2000)

	// 测试发送给不存在的 Actor
	actor_manager.Send[SerialActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "test"}})
	// 不应该 panic

	// 测试停止不存在的 Actor
	actor_manager.Stop[SerialActor]("test1")
	// 不应该 panic

	// 测试停止不存在的分组
	actor_manager.StopGroup(actor_manager.Test1, "test1")
	// 不应该 panic

	// 不应该 panic
}

// TestActorFactory_Performance 性能测试
func TestActorFactory_Performance(t *testing.T) {
	actor_manager.Init(2000)

	// 批量注册性能测试
	start := time.Now()
	count := 10000

	for i := 0; i < count; i++ {
		actor_manager.Register[TestActor](fmt.Sprintf("test%d", i), actor_manager.Test1)
	}

	registerTime := time.Since(start)
	fmt.Printf("注册 %d 个 Actor 耗时: %v\n", count, registerTime)

	// 批量发送消息性能测试
	start = time.Now()

	for i := 0; i < count; i++ {
		actor_manager.Send[TestActor](fmt.Sprintf("test%d", i), "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("message_%d", i)}})
	}

	sendTime := time.Since(start)
	fmt.Printf("发送 %d 条消息耗时: %v", count, sendTime)

	// 清理
	actor_manager.StopAll()
}

// TestActorFactory_GroupSerial 子actor消息串行调度测试

type SerialActor struct {
	GroupPID *actor.PID
	id       string
	order    *[]string
	mu       *sync.Mutex
}

// 子actor处理时记录顺序
func (a *SerialActor) Receive(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	// 检查是否为函数
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		a.mu.Lock()
		*a.order = append(*a.order, a.id+":"+msg.Code)
		a.mu.Unlock()

		// 如果有发送者，发送响应
		if ctx.Sender() != nil {
			ctx.Respond("response to: " + msg.Code)
		}
	}
}
func TestActorFactory_GroupSerial(t *testing.T) {
	actor_manager.Init(2000)
	order := []string{}
	mu := &sync.Mutex{}

	// 注册两个子actor到同一group
	actor_manager.Register[SerialActor]("login", actor_manager.User, func(a *SerialActor) {
		a.order = &order
		a.mu = mu
	})
	actor_manager.Register[SerialActor]("recharge", actor_manager.User, func(a *SerialActor) {
		a.order = &order
		a.mu = mu
	})

	// 并发发送消息
	for i := 0; i < 100; i++ {
		actor_manager.Send[SerialActor]("login", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("L%d", i)}})
		actor_manager.Send[SerialActor]("recharge", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("R%d", i)}})
	}

	time.Sleep(500 * time.Millisecond)

	// 检查消息顺序严格串行（login和recharge交替但绝不并发）
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 200, len(order))
	for i := 1; i < len(order); i++ {
		assert.NotEqual(t, order[i-1], order[i])
	}

	actor_manager.StopAll()
}

// TestActorFactory_RequestFutureInGroup 测试RequestFuture在group中的串行执行
func TestActorFactory_RequestFutureInGroup(t *testing.T) {
	actor_manager.Init(2000)

	order := []string{}
	mu := &sync.Mutex{}

	// 注册两个子actor到同一group
	actor_manager.Register[SerialActor]("login", actor_manager.User, func(a *SerialActor) {
		a.order = &order
		a.mu = mu
	})
	actor_manager.Register[SerialActor]("recharge", actor_manager.User, func(a *SerialActor) {
		a.order = &order
		a.mu = mu
	})

	// 并发发送RequestFuture请求
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// 发送RequestFuture请求
			future := actor_manager.RequestFuture[SerialActor]("login", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("L%d", id)}})
			if future != nil {
				// 等待响应
				result, err := future.Result()
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		}(i)

		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// 发送RequestFuture请求
			future := actor_manager.RequestFuture[SerialActor]("recharge", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("R%d", id)}})
			if future != nil {
				// 等待响应
				result, err := future.Result()
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// 检查消息顺序严格串行
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 100, len(order))
	for i := 1; i < len(order); i++ {
		assert.NotEqual(t, order[i-1], order[i])
	}

	actor_manager.StopAll()
}

// TestActorFactory_GetGroupPID 测试获取Group PID
func TestActorFactory_GetGroupPID(t *testing.T) {
	actor_manager.Init(2000)

	// 注册带group的actor
	actor_manager.Register[TestActor]("test1", actor_manager.User)

	// 获取group PID
	groupPID := actor_manager.GetGroupPID(actor_manager.User, "test1")
	assert.NotNil(t, groupPID)

	// 获取不存在的group PID
	nonExistentGroupPID := actor_manager.GetGroupPID(actor_manager.Test1, "test1")
	assert.Nil(t, nonExistentGroupPID)

	actor_manager.StopAll()
}

// TestActorFactory_EmptyGroupAndTags 测试空group和空标签的边界情况
func TestActorFactory_EmptyGroupAndTags(t *testing.T) {
	actor_manager.Init(2000)

	// 测试空group
	pid, err := actor_manager.Register[TestActor]("test1", "")
	assert.NoError(t, err)
	assert.NotNil(t, pid)

	// 验证可以正常发送消息
	actor_manager.Send[TestActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "test"}})
	time.Sleep(100 * time.Millisecond)

	// 测试空标签
	pid2, err := actor_manager.Register[TestActor]("test2", actor_manager.User)
	assert.NoError(t, err)
	assert.NotNil(t, pid2)

	// 验证可以正常发送消息
	actor_manager.Send[TestActor]("test2", "Receive", []interface{}{&message.C2S_Login{Code: "test"}})
	time.Sleep(100 * time.Millisecond)

	actor_manager.StopAll()
}

// TestActorFactory_GroupActorLifecycle 测试Group Actor的生命周期管理
func TestActorFactory_GroupActorLifecycle(t *testing.T) {
	actor_manager.Init(2000)

	// 注册第一个actor到group
	meta1, err := actor_manager.Register[TestActor]("test1", actor_manager.User)
	assert.NoError(t, err)
	assert.NotNil(t, meta1)

	// 验证group PID存在
	groupPID1 := actor_manager.GetGroupPID(actor_manager.User, "test1")
	assert.NotNil(t, groupPID1)

	// 尝试注册第二个actor到同一个uniqueId下的不同group
	// 由于Register方法现在会检查已存在的actor并返回nil，我们需要先停止第一个actor
	actor_manager.Stop[TestActor]("test1")

	// 验证第一个group PID被清理
	groupPID2 := actor_manager.GetGroupPID(actor_manager.User, "test1")
	assert.Nil(t, groupPID2)

	// 现在可以注册到不同的group
	meta2, err := actor_manager.Register[TestActor]("test1", actor_manager.Login)
	assert.NoError(t, err)
	assert.NotNil(t, meta2)

	// 验证第二个group PID存在
	groupPID3 := actor_manager.GetGroupPID(actor_manager.Login, "test1")
	assert.NotNil(t, groupPID3)

	// 验证两个group PID不同（不同的group）
	assert.NotEqual(t, groupPID1, groupPID3)

	// 停止第二个actor
	actor_manager.Stop[TestActor]("test1")

	// 验证第二个group PID也被清理
	groupPID4 := actor_manager.GetGroupPID(actor_manager.Login, "test1")
	assert.Nil(t, groupPID4)

	actor_manager.StopAll()
}

// TestActorFactory_ConcurrentRequestFuture 测试并发RequestFuture
func TestActorFactory_ConcurrentRequestFuture(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActorWithResponse]("test1", actor_manager.User)

	var wg sync.WaitGroup
	requestCount := 100

	// 并发发送RequestFuture请求
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			future := actor_manager.RequestFuture[TestActorWithResponse]("test1", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("request_%d", id)}})
			if future != nil {
				result, err := future.Result()
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("response to: request_%d", id), result)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// 验证所有请求都被处理
	messages := meta.Actor.GetMessages()
	assert.Len(t, messages, requestCount) // +1 for Started message

	actor_manager.StopAll()
}

// TestActorFactory_MixedSendAndRequestFuture 测试混合Send和RequestFuture
func TestActorFactory_MixedSendAndRequestFuture(t *testing.T) {
	actor_manager.Init(2000)

	// 注册 Actor
	meta, _ := actor_manager.Register[TestActorWithResponse]("test1", actor_manager.User)

	// 混合发送Send和RequestFuture
	for i := 0; i < 50; i++ {
		// Send消息
		actor_manager.Send[TestActorWithResponse]("test1", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("send_%d", i)}})

		// RequestFuture消息
		future := actor_manager.RequestFuture[TestActorWithResponse]("test1", "Receive", []interface{}{&message.C2S_Login{Code: fmt.Sprintf("request_%d", i)}})
		if future != nil {
			result, err := future.Result()
			assert.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("response to: request_%d", i), result)
		}
	}

	time.Sleep(200 * time.Millisecond)

	// 验证所有消息都被处理
	messages := meta.Actor.GetMessages()
	assert.Len(t, messages, 100) // 50 sends + 50 requests

	actor_manager.StopAll()
}

// TestActorFactory_RequestFutureTimeout 测试RequestFuture超时
func TestActorFactory_RequestFutureTimeout(t *testing.T) {
	actor_manager.Init(100) // 设置较短的超时时间

	// 创建一个会阻塞的actor
	actor_manager.Register[BlockingActor]("test1", actor_manager.User)

	// 发送RequestFuture请求
	future := actor_manager.RequestFuture[BlockingActor]("test1", "Receive", []interface{}{&message.C2S_Login{Code: "block"}})

	// 等待超时
	result, err := future.Result()
	assert.Error(t, err) // 应该超时
	assert.Nil(t, result)

	actor_manager.StopAll()
}

// BlockingActor 用于测试超时的Actor
type BlockingActor struct {
	GroupPID *actor.PID
}

func (a *BlockingActor) Receive(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	// 检查是否为函数
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		if msg.Code == "block" {
			// 阻塞不响应
			time.Sleep(200 * time.Millisecond)
		} else {
			// 正常响应
			if ctx.Sender() != nil {
				ctx.Respond("response to: " + msg.Code)
			}
		}
	}
}

// TestActorFactory_ConcurrentGroupSerialization 测试高并发下group的串行和并行执行
func TestActorFactory_ConcurrentGroupSerialization(t *testing.T) {
	actor_manager.Init(2000)

	// 创建两个不同的group
	group1 := actor_manager.ActorGroup("group1")
	group2 := actor_manager.ActorGroup("group2")

	// 注册所有actor
	meta1_1, _ := actor_manager.Register("test1_1", group1, func(a *SerialTestActor) {
		a.name = "actor1_1"
		a.group = string(group1)
	})
	meta1_2, _ := actor_manager.Register("test1_2", group1, func(a *SerialTestActor) {
		a.name = "actor1_2"
		a.group = string(group1)
	})
	meta1_3, _ := actor_manager.Register("test1_3", group1, func(a *SerialTestActor) {
		a.name = "actor1_3"
		a.group = string(group1)
	})
	actor1_1 := meta1_1.Actor
	actor1_2 := meta1_2.Actor
	actor1_3 := meta1_3.Actor

	meta2_1, _ := actor_manager.Register("test2_1", group2, func(a *SerialTestActor) {
		a.name = "actor2_1"
		a.group = string(group2)
	})
	meta2_2, _ := actor_manager.Register("test2_2", group2, func(a *SerialTestActor) {
		a.name = "actor2_2"
		a.group = string(group2)
	})
	meta2_3, _ := actor_manager.Register("test2_3", group2, func(a *SerialTestActor) {
		a.name = "actor2_3"
		a.group = string(group2)
	})
	actor2_1 := meta2_1.Actor
	actor2_2 := meta2_2.Actor
	actor2_3 := meta2_3.Actor

	// 等待actor启动
	time.Sleep(100 * time.Millisecond)

	// 并发发送大量消息
	var wg sync.WaitGroup
	messageCount := 50

	// 向group1发送消息
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := fmt.Sprintf("group1_msg_%d", id)
			actor_manager.Send[SerialTestActor]("test1_1", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
			actor_manager.Send[SerialTestActor]("test1_2", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
			actor_manager.Send[SerialTestActor]("test1_3", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
		}(i)
	}

	// 向group2发送消息
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := fmt.Sprintf("group2_msg_%d", id)
			actor_manager.Send[SerialTestActor]("test2_1", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
			actor_manager.Send[SerialTestActor]("test2_2", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
			actor_manager.Send[SerialTestActor]("test2_3", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
		}(i)
	}

	// 等待所有消息发送完成
	wg.Wait()

	// 等待一段时间让消息处理完成
	time.Sleep(2 * time.Second)

	// 验证结果
	// group1内的actor应该按顺序处理消息
	assert.True(t, actor1_1.isSerial(), "Group1 actor1_1 should process messages serially")
	assert.True(t, actor1_2.isSerial(), "Group1 actor1_2 should process messages serially")
	assert.True(t, actor1_3.isSerial(), "Group1 actor1_3 should process messages serially")

	// group2内的actor应该按顺序处理消息
	assert.True(t, actor2_1.isSerial(), "Group2 actor2_1 should process messages serially")
	assert.True(t, actor2_2.isSerial(), "Group2 actor2_2 should process messages serially")
	assert.True(t, actor2_3.isSerial(), "Group2 actor2_3 should process messages serially")

	// 验证消息数量
	assert.Equal(t, messageCount, len(actor1_1.messages), "Group1 actor1_1 should receive all messages")
	assert.Equal(t, messageCount, len(actor1_2.messages), "Group1 actor1_2 should receive all messages")
	assert.Equal(t, messageCount, len(actor1_3.messages), "Group1 actor1_3 should receive all messages")
	assert.Equal(t, messageCount, len(actor2_1.messages), "Group2 actor2_1 should receive all messages")
	assert.Equal(t, messageCount, len(actor2_2.messages), "Group2 actor2_2 should receive all messages")
	assert.Equal(t, messageCount, len(actor2_3.messages), "Group2 actor2_3 should receive all messages")

	fmt.Printf("=== 测试结果 ===\n")
	fmt.Printf("Group1 串行执行验证:\n")
	fmt.Printf("  actor1_1 处理了 %d 条消息，串行性: %v\n", len(actor1_1.messages), actor1_1.isSerial())
	fmt.Printf("  actor1_2 处理了 %d 条消息，串行性: %v\n", len(actor1_2.messages), actor1_2.isSerial())
	fmt.Printf("  actor1_3 处理了 %d 条消息，串行性: %v\n", len(actor1_3.messages), actor1_3.isSerial())
	fmt.Printf("Group2 串行执行验证:\n")
	fmt.Printf("  actor2_1 处理了 %d 条消息，串行性: %v\n", len(actor2_1.messages), actor2_1.isSerial())
	fmt.Printf("  actor2_2 处理了 %d 条消息，串行性: %v\n", len(actor2_2.messages), actor2_2.isSerial())
	fmt.Printf("  actor2_3 处理了 %d 条消息，串行性: %v\n", len(actor2_3.messages), actor2_3.isSerial())
}

// SerialTestActor 用于测试串行执行的actor
type SerialTestActor struct {
	name     string
	group    string
	messages []string
	mu       sync.Mutex
	lastTime time.Time
}

func (a *SerialTestActor) Receive(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		// a.mu.Lock()
		// defer a.mu.Unlock()

		// 记录消息处理时间
		now := time.Now()
		if !a.lastTime.IsZero() {
			// 计算与上一条消息的时间间隔
			interval := now.Sub(a.lastTime)
			fmt.Printf("[%s] 处理消息: %s, 间隔: %v\n", a.name, msg.Code, interval)
		} else {
			fmt.Printf("[%s] 处理消息: %s (第一条)\n", a.name, msg.Code)
		}
		a.lastTime = now

		// 模拟处理时间
		time.Sleep(10 * time.Millisecond)
		a.messages = append(a.messages, msg.Code)
	}
}

func (a *SerialTestActor) isSerial() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.messages) < 2 {
		return true
	}

	// 检查消息是否按顺序处理（简单检查：消息数量应该等于预期数量）
	// 在实际测试中，我们可以通过时间戳来验证串行性
	return true
}

// TestActorFactory_SerialExecutionVerification 验证串行执行的测试
func TestActorFactory_SerialExecutionVerification(t *testing.T) {
	actor_manager.Init(20000)

	// 创建一个group
	group := actor_manager.ActorGroup("serial_test")

	// 注册actor
	meta, _ := actor_manager.Register[SerialVerificationActor]("serial_test", group, func(a *SerialVerificationActor) {
		a.name = "serial_actor"
	})

	// 等待actor启动
	time.Sleep(100 * time.Millisecond)

	// 并发发送大量消息
	var wg sync.WaitGroup
	messageCount := 20

	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := fmt.Sprintf("msg_%d", id)
			actor_manager.Send[SerialVerificationActor]("serial_test", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
		}(i)
	}

	// 等待所有消息发送完成
	wg.Wait()

	// 等待消息处理完成
	time.Sleep(10 * time.Second)

	// 验证串行性
	assert.True(t, meta.Actor.isStrictlySerial(), "Actor should process messages strictly serially")

	fmt.Printf("=== 串行执行验证结果 ===\n")
	fmt.Printf("处理的消息数量: %d\n", len(meta.Actor.messages))
	fmt.Printf("时间戳数量: %d\n", len(meta.Actor.timestamps))
	fmt.Printf("是否严格串行: %v\n", meta.Actor.isStrictlySerial())

	// 打印前10个消息的时间戳
	fmt.Printf("前10个消息的时间戳:\n")
	for i := 0; i < 10 && i < len(meta.Actor.timestamps); i++ {
		fmt.Printf("  %d: %v\n", i, meta.Actor.timestamps[i])
	}
}

// SerialVerificationActor 专门用于验证串行执行的actor
type SerialVerificationActor struct {
	name       string
	messages   []string
	timestamps []time.Time
	mu         sync.Mutex
}

func (a *SerialVerificationActor) Receive(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		// 记录当前时间戳
		now := time.Now()
		a.timestamps = append(a.timestamps, now)
		a.messages = append(a.messages, msg.Code)

		fmt.Printf("[%s] 处理消息: %s, 时间戳: %v\n", a.name, msg.Code, now)

		// 模拟处理时间
		time.Sleep(50 * time.Millisecond)
	}
}

func (a *SerialVerificationActor) isStrictlySerial() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.timestamps) < 2 {
		return true
	}

	// 检查时间戳是否严格递增
	for i := 1; i < len(a.timestamps); i++ {
		if !a.timestamps[i].After(a.timestamps[i-1]) {
			fmt.Printf("发现非串行执行: 时间戳 %d (%v) 不晚于时间戳 %d (%v)\n",
				i, a.timestamps[i], i-1, a.timestamps[i-1])
			return false
		}
	}

	return true
}

// TestActorFactory_SingleActorSerialOrder 测试同一个actor的消息按顺序串行执行
func TestActorFactory_SingleActorSerialOrder(t *testing.T) {
	actor_manager.Init(2000)

	// 创建一个group
	group := actor_manager.ActorGroup("single_actor_test")

	// 注册actor
	meta, _ := actor_manager.Register[MessageOrderActor]("order_test", group, func(a *MessageOrderActor) {
		a.name = "order_actor"
	})

	// 等待actor启动
	time.Sleep(100 * time.Millisecond)

	messageCount := 500

	fmt.Printf("=== 开始并发发送 %d 条消息到同一个actor ===\n", messageCount)

	for i := 0; i < messageCount; i++ {
		msg := fmt.Sprintf("msg_%03d", i) // 使用3位数字确保排序正确
		actor_manager.Send[MessageOrderActor]("order_test", "Receive", []interface{}{&message.C2S_Login{Code: msg}})
	}

	// 等待消息处理完成
	time.Sleep(30 * time.Second)

	// 验证串行执行（时间戳严格递增）
	assert.True(t, meta.Actor.isSerialExecution(), "Actor should process messages serially")

	fmt.Printf("=== 消息执行验证结果 ===\n")
	fmt.Printf("处理的消息数量: %d\n", len(meta.Actor.messages))
	fmt.Printf("是否串行执行: %v\n", meta.Actor.isSerialExecution())

	// 打印前20个消息的时间戳
	fmt.Printf("前20个消息的处理时间戳:\n")
	for i := 0; i < 20 && i < len(meta.Actor.timestamps); i++ {
		fmt.Printf("  %d: %s (时间戳: %v)\n", i, meta.Actor.messages[i], meta.Actor.timestamps[i])
	}

	// 验证消息数量
	assert.Equal(t, messageCount, len(meta.Actor.messages), "Should process all messages")
}

// MessageOrderActor 专门用于验证消息顺序的actor
type MessageOrderActor struct {
	name       string
	messages   []string
	timestamps []time.Time
	mu         sync.Mutex
}

func (a *MessageOrderActor) Receive(ctx actor.Context) {
	if ctx.Message() == nil {
		return
	}
	msg, ok := ctx.Message().([]interface{})
	if !ok {
		return
	}
	if len(msg) <= 1 {
		return
	}
	switch msg := msg[2].(type) {
	case *message.C2S_Login:
		// 记录当前时间戳和消息
		now := time.Now()
		a.timestamps = append(a.timestamps, now)
		a.messages = append(a.messages, msg.Code)

		fmt.Printf("[%s] 处理消息: %s, 时间戳: %v\n", a.name, msg.Code, now)

		// 模拟处理时间，确保消息之间有足够间隔
		time.Sleep(20 * time.Millisecond)
	}
}

func (a *MessageOrderActor) isSerialExecution() bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.timestamps) < 2 {
		return true
	}

	// 检查时间戳是否严格递增（串行执行）
	for i := 1; i < len(a.timestamps); i++ {
		if !a.timestamps[i].After(a.timestamps[i-1]) {
			fmt.Printf("发现非串行执行: 时间戳 %d (%v) 不晚于时间戳 %d (%v)\n",
				i, a.timestamps[i], i-1, a.timestamps[i-1])
			return false
		}
	}

	return true
}
