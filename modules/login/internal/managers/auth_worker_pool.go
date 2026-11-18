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
	ctx           context.Context
	cancel        context.CancelFunc

	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
	done   chan struct{}
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

	ctx, cancel := context.WithCancel(context.Background())
	p := &authWorkerPool{
		authenticator: authenticator,
		timeout:       timeout,
		queue:         make(chan authTask, queueSize),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
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
		var response *models.LoginResponse
		if p.ctx.Err() == nil {
			ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
			response = p.authenticator.Authenticate(ctx, task.agent, task.request)
			cancel()
		}
		if p.ctx.Err() != nil {
			response = nil
		}
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

// close 停止接收新任务并等待排空；等待超时后取消在途认证，排队任务以失败结束。
// 超时只结束本次等待，调用方仍可用新的上下文等待全部工作协程退出。
func (p *authWorkerPool) close(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.queue)
		go func() { p.wg.Wait(); p.cancel(); close(p.done) }()
	}
	p.mu.Unlock()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}
