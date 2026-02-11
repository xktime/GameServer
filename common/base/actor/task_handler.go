package actor

import (
	"context"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/core/log"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type TaskQueue struct {
	f        func() *Response
	response chan *Response
}

type Response struct {
	Result []interface{} // 改为具体类型，避免使用 interface{} 指针
	Error  error         // 添加错误字段
}

type ActorState int

const (
	None ActorState = iota
	ActorStateRunning
	ActorStateStopping
	ActorStateStopped
)

// TaskHandler 提供通用的Actor实现
type TaskHandler struct {
	taskQueue chan *TaskQueue
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	id        string
	actors    map[string]IActor
	state     ActorState
	mu        sync.RWMutex // 保护状态和actors的并发访问
	// 用于检测重入调用
	processingGoroutineID int64      // 当前正在处理的goroutine ID
	muReentrant           sync.Mutex // 保护重入检测的锁
}

// RegisterActor 泛型初始化方法，自动创建和缓存Actor对象
// T 必须实现 IActor 接口，函数总是返回指向T的指针
func RegisterActor[T IActor](actorGroup ActorGroup, uniqueID interface{}, args ...any) T {
	// 创建泛型类型的零值
	var zero T
	actorType := reflect.TypeOf(zero)

	// 创建实例
	var actor T
	if actorType.Kind() == reflect.Ptr {
		// 如果T已经是指针类型，我们需要创建指向T指向类型的实例
		elemType := actorType.Elem()
		actorValue := reflect.New(elemType)
		// 将指针转换为T类型（T是指针类型，如*TeamManager）
		actor = actorValue.Interface().(T)
	} else {
		// 如果T是值类型，创建指向T的指针
		actorValue := reflect.New(actorType)
		// 将指针转换为T类型
		actor = actorValue.Interface().(T)
	}

	// 转换为IActor接口
	instance := any(actor)
	actorInterface, ok := instance.(IActor)
	if !ok {
		panic(fmt.Sprintf("类型 %T 没有实现 IActor 接口", actor))
	}

	// 获取或创建TaskHandler
	taskHandler := getTaskHandler(actorGroup, uniqueID, actorInterface)

	// 设置TaskHandler到Actor中
	actorInterface.SetTaskHandler(taskHandler)

	// 初始化Actor
	actorInterface.Init(args...)

	// 初始化判断一下跨天
	if timer, ok := actorInterface.(OnCrossDayTimer); ok {
		timer.OnCrossDay(time.Now().Unix())
	}
	return actor
}

// GetActor 获取已存在的Actor对象
func GetActor[T any](actorGroup ActorGroup, uniqueID interface{}) (*T, bool) {
	id := getUniqueId(actorGroup, uniqueID)
	handler, exists := GetHandler(id)
	if !exists || handler.IsStopped() {
		return nil, false
	}

	// 安全的类型断言
	name := getActorNameByType[T]()
	if actor, ok := handler.actors[name]; ok {
		// 直接进行类型断言，返回原始引用
		if result, ok := actor.(T); ok {
			return &result, true
		}

		// 如果类型不匹配，尝试指针类型转换
		actorValue := reflect.ValueOf(actor)
		var zero T
		zeroType := reflect.TypeOf(zero)

		// 如果存储的是指针类型，且指向的类型匹配
		if actorValue.Kind() == reflect.Ptr && actorValue.Type().Elem() == zeroType {
			// 直接返回原始指针的引用
			return actorValue.Interface().(*T), true
		}

		// 如果存储的是值类型，创建指向原始值的指针
		if actorValue.Type() == zeroType {
			// 获取原始值的地址并转换为目标类型
			ptr := reflect.NewAt(zeroType, unsafe.Pointer(actorValue.UnsafeAddr()))
			return ptr.Interface().(*T), true
		}
	}

	return nil, false
}

// StopActor 停止指定的Actor
func StopActor[T IActor](actorGroup ActorGroup, uniqueID interface{}) error {
	id := getUniqueId(actorGroup, uniqueID)
	handler, exists := GetHandler(id)
	if !exists {
		return fmt.Errorf("actor not found: %s", id)
	}

	// 获取Actor类型名称
	name := getActorNameByType[T]()
	if actor, ok := handler.actors[name]; ok {
		// 停止Actor
		actor.Stop()

		// 保存数据（如果实现了PersistData接口）
		if persistData, ok := actor.(mongodb.PersistData); ok {
			if _, err := mongodb.Save(persistData); err != nil {
				log.Error("停止Actor时保存失败: %v", err)
			}
		}

		// 从TaskHandler中移除
		handler.RemoveActor(actor)
	}

	return nil
}

// 获取泛型T对应的collection名称
func getActorNameByType[T any]() string {
	var t T
	return getActorName(t)
}

func getActorName(a any) string {
	typ := reflect.TypeOf(a)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}

func getUniqueId(ActorGroup ActorGroup, uniqueID interface{}) string {
	return fmt.Sprintf("%s_%v", ActorGroup, uniqueID)
}

// getGoroutineID 获取当前goroutine的ID
func getGoroutineID() int64 {
	buf := make([]byte, 64)
	n := runtime.Stack(buf, false)
	// 从堆栈信息中提取goroutine ID
	var id int64
	fmt.Sscanf(string(buf[:n]), "goroutine %d", &id)
	return id
}

// SendTask 发送任务到队列，支持超时机制
func (b *TaskHandler) sendTask(f func() *Response, isReentrant bool) *Response {
	// 检查context是否已取消
	if b.ctx.Err() != nil {
		return &Response{
			Result: nil,
			Error:  b.ctx.Err(),
		}
	}

	if isReentrant {
		// 重入调用，直接执行任务，避免死锁
		return f()
	}

	// 检查是否已停止或正在停止
	state := b.GetState()
	if state != ActorStateRunning {
		return &Response{
			Result: nil,
			Error:  fmt.Errorf("actor is not accepting new tasks, current state: %v", state),
		}
	}

	task := &TaskQueue{
		f:        f,
		response: make(chan *Response, 1),
	}

	select {
	case b.taskQueue <- task:
		// 任务发送成功，设置超时
		timeout := time.Duration(GetTaskTimeout()) * time.Millisecond
		select {
		case result := <-task.response:
			return result
		case <-time.After(timeout):
			// 超时，打印调用栈
			b.printCallStack("SendTask timeout")
			return &Response{
				Result: nil,
				Error:  fmt.Errorf("task execution timeout after %v", timeout),
			}
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

// SendTask 发送任务并等待结果
// 自动处理Response构造和结果提取，支持任意返回值
// 如果函数没有返回值，则直接进入队列执行，不等待返回结果
func (b *TaskHandler) SendTask(f interface{}) interface{} {
	// 使用反射调用函数并自动处理结果
	fn := reflect.ValueOf(f)
	fnType := fn.Type()

	// 检查函数签名
	if fnType.Kind() != reflect.Func {
		panic("SendTask: 参数必须是函数")
	}

	// 检测重入调用
	currentGoroutineID := getGoroutineID()
	b.muReentrant.Lock()
	isReentrant := b.processingGoroutineID == currentGoroutineID
	b.muReentrant.Unlock()

	// 检查函数是否有返回值
	if fnType.NumOut() == 0 {
		// 无返回值，如果是重入，直接执行；不是，放队列异步执行
		if isReentrant {
			fn.Call([]reflect.Value{})
		} else {
			b.SendTaskAsync(func() {
				fn.Call([]reflect.Value{})
			})
		}
		return &Response{}
	}

	// 调用SendTask，但自动构造Response
	response := b.sendTask(func() *Response {
		// 调用传入的函数
		results := fn.Call([]reflect.Value{})

		// 自动构造Response
		response := &Response{}

		// 处理返回值
		if len(results) == 0 {
			// 无返回值
			return response
		} else if len(results) == 1 {
			// 单个返回值
			if err, ok := results[0].Interface().(error); ok {
				// 返回error
				response.Error = err
			} else {
				// 返回普通值
				response.Result = []interface{}{results[0].Interface()}
			}
		} else {
			// 多个返回值，最后一个可能是error
			lastResult := results[len(results)-1]
			if err, ok := lastResult.Interface().(error); ok && err != nil {
				// 最后一个值是error且不为nil
				response.Error = err
				response.Result = make([]interface{}, len(results)-1)
				for i := 0; i < len(results)-1; i++ {
					response.Result[i] = results[i].Interface()
				}
			} else {
				// 所有值都是普通返回值
				response.Result = make([]interface{}, len(results))
				for i, result := range results {
					response.Result[i] = result.Interface()
				}
			}
		}

		return response
	}, isReentrant)

	// 自动提取结果
	if response.Error != nil {
		return response.Error
	}

	if len(response.Result) == 0 {
		return nil
	} else if len(response.Result) == 1 {
		return response.Result[0]
	} else {
		return response.Result
	}
}

// SendTaskAsync 异步发送任务，不等待结果，不阻塞执行
func (b *TaskHandler) SendTaskAsync(f func()) bool {
	// 检查是否已停止或正在停止
	state := b.GetState()
	if state != ActorStateRunning {
		return false
	}

	// 检查context是否已取消
	if b.ctx.Err() != nil {
		return false
	}

	task := &TaskQueue{
		f:        func() *Response { f(); return nil },
		response: make(chan *Response, 1),
	}

	select {
	case b.taskQueue <- task:
		// 任务发送成功，不等待结果
		return true
	case <-b.ctx.Done():
		// Actor已停止
		return false
	default:
		// 任务队列已满，不阻塞
		return false
	}
}

func (b *TaskHandler) Start() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查状态，避免重复启动
	if b.state == ActorStateRunning {
		return
	}

	// 如果正在停止或已停止，不能启动
	if b.state == ActorStateStopping || b.state == ActorStateStopped {
		return
	}

	b.state = ActorStateRunning

	// 注册到Actor管理器
	if b.id != "" {
		Register(b.id, b)
	}

	b.wg.Add(1)
	go b.Processor()
}

func (b *TaskHandler) Stop() {
	b.mu.Lock()

	// 检查状态，避免重复停止
	if b.state == ActorStateStopped {
		b.mu.Unlock()
		return
	}

	// 如果已经在停止中，等待完成
	if b.state == ActorStateStopping {
		b.mu.Unlock()
		return
	}

	// 标记为正在停止（此时不再接受新任务，但队列中的任务继续执行）
	b.state = ActorStateStopping

	// 关闭任务队列（不再接受新任务）
	close(b.taskQueue)

	// 释放锁，等待所有goroutine完成（包括处理队列中剩余的任务）
	b.mu.Unlock()
	b.wg.Wait()

	// 重新获取锁以完成剩余的清理工作
	b.mu.Lock()

	// 现在取消context（此时所有任务已完成）
	b.cancel()

	// 在Stop时保存所有Actor数据
	for _, actor := range b.actors {
		if persistData, ok := actor.(mongodb.PersistData); ok {
			if _, err := mongodb.Save(persistData); err != nil {
				// 这里简单打印日志，实际可根据需要处理
				log.Error("Actor关闭保存失败: %v", err)
			}
		}
	}

	// 清理所有 Actor 引用
	b.actors = make(map[string]IActor)

	// 标记为已停止
	b.state = ActorStateStopped

	// 释放锁，以便Unregister可以获取锁
	b.mu.Unlock()

	// 从Actor管理器注销
	if b.id != "" {
		Unregister(b.id)
	}
}

func (b *TaskHandler) Processor() {
	defer func() {
		if r := recover(); r != nil {
			log.Error("Processor panic: %v", r)
			b.printCallStack("Processor panic")
			// 通知所有等待的任务执行失败
			b.notifyAllTasksFailed(fmt.Errorf("processor panic: %v", r))
		}
		b.wg.Done()
	}()

	for {
		select {
		case task, ok := <-b.taskQueue:
			if !ok {
				// taskQueue 已关闭，处理完剩余任务后退出
				// 继续处理队列中剩余的任务
				for remainingTask := range b.taskQueue {
					if remainingTask != nil {
						b.processTask(remainingTask)
					}
				}
				return
			}
			if task != nil {
				b.processTask(task)
			}
		case <-b.ctx.Done():
			// 只有在非停止状态下才响应context取消
			if b.GetState() == ActorStateRunning {
				return
			}
			// 在停止状态下，继续处理剩余任务
		}
	}
}

// 添加从 TaskHandler 中移除特定 Actor 的方法
func (b *TaskHandler) RemoveActor(a any) {
	b.mu.Lock()

	// 检查状态，只有在运行状态下才能移除actor
	if b.state != ActorStateRunning {
		b.mu.Unlock()
		return
	}

	actorName := getActorName(a)
	delete(b.actors, actorName)

	// 检查是否还有 Actor
	shouldStop := len(b.actors) == 0
	b.mu.Unlock() // 先释放锁

	// 如果没有 Actor 了，异步停止 TaskHandler（避免死锁）
	if shouldStop {
		b.Stop() // 使用 goroutine 异步停止，避免死锁
	}
}

// GetState 获取当前状态（线程安全）
func (b *TaskHandler) GetState() ActorState {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// IsRunning 检查是否正在运行（线程安全）
func (b *TaskHandler) IsRunning() bool {
	return b.GetState() == ActorStateRunning
}

// IsStopped 检查是否已停止（线程安全）
func (b *TaskHandler) IsStopped() bool {
	return b.GetState() == ActorStateStopped || b.GetState() == ActorStateStopping
}

// notifyAllTasksFailed 通知所有等待的任务执行失败
func (b *TaskHandler) notifyAllTasksFailed(err error) {
	// 这里可以实现更复杂的通知机制
	// 目前简单记录错误
	log.Error("All pending tasks failed: %v", err)
}

// GetQueueLength 获取当前队列中的任务数量
func (b *TaskHandler) GetQueueLength() int {
	return len(b.taskQueue)
}

// IsStopping 检查是否正在停止（队列中可能还有任务在执行）
func (b *TaskHandler) IsStopping() bool {
	return b.GetState() == ActorStateStopping
}

// GetTaskHandler 原有的初始化方法，保持向后兼容
func getTaskHandler(ActorGroup ActorGroup, uniqueID interface{}, a IActor) *TaskHandler {
	id := getUniqueId(ActorGroup, uniqueID)
	actorName := getActorName(a)
	if taskHandler, ok := GetHandler(id); ok && !taskHandler.IsStopped() {
		taskHandler.actors[actorName] = a
		return taskHandler
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		h := &TaskHandler{
			taskQueue: make(chan *TaskQueue, 10000),
			ctx:       ctx,
			cancel:    cancel,
			id:        id,
			actors:    make(map[string]IActor),
		}
		h.actors[actorName] = a
		h.Start()
		return h
	}
}

// printCallStack 打印调用栈信息
func (b *TaskHandler) printCallStack(reason string) {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	log.Error("%s - Call stack:\n%s", reason, string(buf[:n]))
}

// processTask 处理单个任务
func (b *TaskHandler) processTask(task *TaskQueue) {
	// 设置当前正在处理的goroutine ID
	currentGoroutineID := getGoroutineID()
	b.muReentrant.Lock()
	b.processingGoroutineID = currentGoroutineID
	b.muReentrant.Unlock()

	// 执行任务
	var result *Response
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("Task execution panic: %v", r)
				b.printCallStack("Task execution panic")
				result = &Response{
					Result: nil,
					Error:  fmt.Errorf("task execution panic: %v", r),
				}
			}
		}()
		result = task.f()
	}()

	// 清除正在处理的goroutine ID
	b.muReentrant.Lock()
	b.processingGoroutineID = 0
	b.muReentrant.Unlock()

	// 确保结果不为nil
	if result == nil {
		result = &Response{
			Result: nil,
			Error:  fmt.Errorf("task returned nil result"),
		}
	}

	// 发送结果
	select {
	case task.response <- result:
	default:
		// 如果响应通道已满，丢弃结果
		log.Error("Task response channel is full, dropping result")
	}
}
