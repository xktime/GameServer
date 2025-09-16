package actor

import "fmt"

type IActor interface {
	Init(args ...any)
	Stop()
	SetTaskHandler(handler *TaskHandler)
}

// BaseActor 提供通用的Actor基础实现
type BaseActor struct {
	TaskHandler *TaskHandler
}

// SetTaskHandler 设置TaskHandler，所有嵌入BaseActor的结构体都会自动获得此方法
func (b *BaseActor) SetTaskHandler(handler *TaskHandler) {
	b.TaskHandler = handler
}

// GetTaskHandler 获取TaskHandler
func (b *BaseActor) GetTaskHandler() *TaskHandler {
	return b.TaskHandler
}

// SendTask 发送任务并等待结果
func (b *BaseActor) SendTask(f func() *Response) *Response {
	if b.TaskHandler == nil {
		return &Response{Error: fmt.Errorf("TaskHandler is nil")}
	}
	return b.TaskHandler.SendTask(f)
}

// SendTaskAsync 异步发送任务，不等待结果
func (b *BaseActor) SendTaskAsync(f func() *Response) bool {
	if b.TaskHandler == nil {
		return false
	}
	return b.TaskHandler.SendTaskAsync(f)
}

// RemoveActor 从TaskHandler中移除Actor
func (b *BaseActor) RemoveActor(actor IActor) {
	if b.TaskHandler != nil {
		b.TaskHandler.RemoveActor(actor)
	}
}

// Stop 停止Actor
func (b *BaseActor) Stop() {
	if b.TaskHandler != nil {
		b.TaskHandler.Stop()
	}
}
