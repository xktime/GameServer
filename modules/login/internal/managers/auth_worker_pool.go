package managers

import (
	"context"
	"errors"
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/modules/game/playersession"
	"gameserver/modules/login/internal/models"
	"gameserver/modules/login/internal/processor"
	"sync"
	"time"
)

const (
	DefaultAuthWorkers   = 32
	DefaultAuthQueueSize = 256
	DefaultAuthTimeout   = 5 * time.Second
)

var (
	ErrAuthQueueFull  = errors.New("login: authentication queue is full")
	ErrAuthPoolClosed = errors.New("login: authentication worker pool is closed")
)

type authTask struct {
	agent    gate.ConnectionAgent
	request  *message.C2S_Login
	attempt  *playersession.Attempt
	complete func(*models.LoginResponse)
}

// authWorkerPool 通过固定数量的工作协程和有界队列限制平台认证的并发量。
type authWorkerPool struct {
	authenticator processor.Authenticator
	timeout       time.Duration
	queue         chan authTask

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func newAuthWorkerPool(authenticator processor.Authenticator, workers, queueSize int, timeout time.Duration) (*authWorkerPool, error) {
	if authenticator == nil {
		return nil, errors.New("login: authenticator is nil")
	}
	if workers <= 0 {
		return nil, errors.New("login: auth worker count must be positive")
	}
	if queueSize <= 0 {
		return nil, errors.New("login: auth queue size must be positive")
	}
	if timeout <= 0 {
		return nil, errors.New("login: auth timeout must be positive")
	}

	p := &authWorkerPool{
		authenticator: authenticator,
		timeout:       timeout,
		queue:         make(chan authTask, queueSize),
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p, nil
}

// worker 为每次平台认证单独设置超时，并在认证返回后调用完成回调。
func (p *authWorkerPool) worker() {
	defer p.wg.Done()
	for task := range p.queue {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		response := p.authenticator.Authenticate(ctx, task.agent, task.request)
		cancel()
		if task.complete != nil {
			task.complete(response)
		}
	}
}

// submit 非阻塞地提交任务；与关闭操作共用锁，避免向已关闭的队列发送。
func (p *authWorkerPool) submit(task authTask) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrAuthPoolClosed
	}
	select {
	case p.queue <- task:
		return nil
	default:
		return ErrAuthQueueFull
	}
}

// close 停止接收新任务，并等待工作协程处理完队列中的任务。
func (p *authWorkerPool) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.queue)
	p.mu.Unlock()
	p.wg.Wait()
}
