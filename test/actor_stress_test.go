//go:build stress

package test

import (
	"fmt"
	"gameserver/common/base/actor"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestNewActorSystem_Performance 性能测试
func TestNewActorSystem_Performance(t *testing.T) {
	actor.Init(2000)

	// 创建 Actor
	testActor := actor.RegisterActor[*NewTestActor](actor.Test1, "test1")

	// 批量发送任务性能测试
	start := time.Now()
	count := 10000

	for i := 0; i < count; i++ {
		testActor.SendTask(func() *actor.Response {
			msg := fmt.Sprintf("message_%d", i)
			testActor.addMessage(msg)
			return &actor.Response{
				Result: []interface{}{msg},
			}
		})
	}

	sendTime := time.Since(start)
	fmt.Printf("发送 %d 个任务耗时: %v\n", count, sendTime)

	// 等待任务处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证所有任务都被处理
	messages := testActor.GetMessages()
	assert.Len(t, messages, count)

	testActor.Stop()
}

// TestNewActorSystem_StressTest 压力测试
func TestNewActorSystem_StressTest(t *testing.T) {
	actor.Init(5000) // 增加队列大小

	// 创建多个Actor进行压力测试
	actorCount := 50
	actors := make([]*NewTestActor, actorCount)

	fmt.Printf("=== 压力测试 ===\n")
	fmt.Printf("创建 %d 个Actor，每个发送100条消息\n", actorCount)

	// 创建Actor
	for i := 0; i < actorCount; i++ {
		actors[i] = actor.RegisterActor[*NewTestActor](actor.Test1, fmt.Sprintf("stress_test_%d", i))
	}

	// 并发发送大量消息
	var wg sync.WaitGroup
	messageCount := 100

	start := time.Now()
	for i := 0; i < actorCount; i++ {
		wg.Add(1)
		go func(actorIndex int) {
			defer wg.Done()
			for j := 0; j < messageCount; j++ {
				actors[actorIndex].SendTask(func() *actor.Response {
					actors[actorIndex].addMessage(fmt.Sprintf("stress_msg_%d_%d", actorIndex, j))
					return &actor.Response{Result: []interface{}{fmt.Sprintf("stress_msg_%d_%d", actorIndex, j)}}
				})
			}
		}(i)
	}

	wg.Wait()
	sendTime := time.Since(start)

	// 等待消息处理完成
	time.Sleep(2 * time.Second)

	// 验证所有消息都被处理
	totalMessages := 0
	for i := 0; i < actorCount; i++ {
		messages := actors[i].GetMessages()
		totalMessages += len(messages)
		assert.Equal(t, messageCount, len(messages), "Actor %d should process all messages", i)
	}

	expectedTotal := actorCount * messageCount
	assert.Equal(t, expectedTotal, totalMessages, "All messages should be processed")

	fmt.Printf("压力测试结果:\n")
	fmt.Printf("  发送时间: %v\n", sendTime)
	fmt.Printf("  总消息数: %d\n", totalMessages)
	fmt.Printf("  吞吐量: %.2f msg/s\n", float64(totalMessages)/sendTime.Seconds())

	// 清理
	for i := 0; i < actorCount; i++ {
		actors[i].Stop()
	}
}
