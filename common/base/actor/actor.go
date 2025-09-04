package actor

import (
	"context"
	"fmt"
	"gameserver/core/log"
	"reflect"
	"sync"
)

type TaskQueue struct {
	f        func() *Response
	response chan *Response
}

type Response struct {
	Result []interface{} // 改为具体类型，避免使用 interface{} 指针
	Error  error         // 添加错误字段
}

// BaseActor 提供通用的Actor实现
type TaskHandler struct {
	taskQueue chan *TaskQueue
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	id        string
	actors    map[string]IActor
}

// InitTaskHandler
func InitTaskHandler(ActorGroup ActorGroup, uniqueID interface{}, a IActor) *TaskHandler {
	id := getUniqueId(ActorGroup, uniqueID)
	actorName := getActorName(a)
	if taskHandler, ok := GetHandler(id); ok {
		taskHandler.actors[actorName] = a
		return taskHandler
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		h := &TaskHandler{
			taskQueue: make(chan *TaskQueue, 1000),
			ctx:       ctx,
			cancel:    cancel,
			id:        id,
			actors:    make(map[string]IActor),
		}
		h.actors[actorName] = a
		return h
	}
}

// 获取泛型T对应的collection名称
func getActorNameByType[T IActor]() string {
	var t T
	return getActorName(t)
}

func getActorName(a IActor) string {
	typ := reflect.TypeOf(a)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}

func getUniqueId(ActorGroup ActorGroup, uniqueID interface{}) string {
	return fmt.Sprintf("%s_%v", ActorGroup, uniqueID)
}

func (b *TaskHandler) SendTask(f func() *Response) *Response {
	// 检查是否已停止
	if b.ctx.Err() != nil {
		return &Response{
			Result: nil,
			Error:  b.ctx.Err(),
		}
	}

	task := &TaskQueue{
		f:        f,
		response: make(chan *Response, 1),
	}

	select {
	case b.taskQueue <- task:
		// 任务发送成功
		select {
		case result := <-task.response:
			return result
		case <-b.ctx.Done():
			return &Response{
				Result: nil,
				Error:  b.ctx.Err(),
			}
		}
	case <-b.ctx.Done():
		// Actor已停止
		return &Response{
			Result: nil,
			Error:  b.ctx.Err(),
		}
	}
}

// 添加从 TaskHandler 中移除特定 Actor 的方法
func (b *TaskHandler) RemoveActor(actorName string) {
	delete(b.actors, actorName)

	// 如果没有 Actor 了，可以考虑停止 TaskHandler
	if len(b.actors) == 0 {
		b.Stop()
	}
}

func (b *TaskHandler) Start() {
	// 注册到Actor管理器
	if b.id != "" {
		Register(b.id, b)
	}

	b.wg.Add(1)
	go b.Processor()
}

func (b *TaskHandler) Stop() {
	b.cancel()
	b.wg.Wait()

	// 清理所有 Actor 引用
	b.actors = make(map[string]IActor)

	// 从Actor管理器注销
	if b.id != "" {
		Unregister(b.id)
	}
}

func (b *TaskHandler) Processor() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("Processor panic: %v", r)
		}
		b.wg.Done()
	}()

	for {
		select {
		case task := <-b.taskQueue:
			if task != nil {
				result := task.f()
				select {
				case task.response <- result:
				case <-b.ctx.Done():
					return
				}
			}
		case <-b.ctx.Done():
			return
		}
	}
}

// 添加优雅关闭方法
func (b *TaskHandler) GracefulStop() {
	// 先停止处理
	b.Stop()

	// 等待所有任务处理完成
	select {
	case <-b.ctx.Done():
		// 已经停止，直接关闭
	default:
		// 等待任务队列清空
		for len(b.taskQueue) > 0 {
			select {
			case task := <-b.taskQueue:
				if task != nil {
					// 处理剩余任务
					result := task.f()
					select {
					case task.response <- result:
					default:
						// 如果响应通道已满，丢弃结果
					}
				}
			default:
				break
			}
		}
	}

	// 关闭任务队列
	close(b.taskQueue)
}
