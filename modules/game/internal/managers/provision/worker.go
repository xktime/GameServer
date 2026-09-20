package provision

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/core/log"
	"gameserver/modules/game/internal/managers/player"
	"gameserver/modules/game/internal/managers/team"
	playermodel "gameserver/modules/game/internal/models/player"
	model "gameserver/modules/game/internal/provision"
	"sync"
	"time"
)

const (
	workerInterval = time.Second
	maxAttempts    = 5
)

// Materializer 根据创建意图幂等地写入领域文档，
// 由调用方负责将意图标记为已应用。
type Materializer interface {
	Apply(model.Intent) error
}

// FailureHandler 在创建意图耗尽重试次数仍无法完成持久化时，
// 终止基于该意图创建的内存会话。
type FailureHandler interface {
	HandleProvisionFailure(context.Context, model.Intent) error
}

type MongoMaterializer struct{}

func (MongoMaterializer) Apply(intent model.Intent) error {
	teamDoc := &team.Team{
		TeamId:      intent.Team.TeamID,
		LeaderId:    intent.Team.LeaderID,
		TeamMembers: append([]int64(nil), intent.Team.MemberIDs...),
		RoomId:      intent.Team.RoomID,
	}
	if _, err := mongodb.SaveIfAbsent(teamDoc); err != nil {
		return fmt.Errorf("save Team %d: %w", intent.Team.TeamID, err)
	}

	playerDoc := &player.Player{
		PlayerId:   intent.Player.PlayerID,
		PlayerInfo: clonePlayerInfo(intent.Player.PlayerInfo),
		TeamId:     intent.Player.TeamID,
		TowerLevel: intent.Player.TowerLevel,
	}
	if _, err := mongodb.SaveIfAbsent(playerDoc); err != nil {
		return fmt.Errorf("save Player %d: %w", intent.Player.PlayerID, err)
	}

	if _, err := mongodb.SaveIfAbsent(&intent.User); err != nil {
		return fmt.Errorf("save User %s: %w", intent.User.AccountId, err)
	}
	return nil
}

// Worker 管理当前进程内玩家创建持久化操作的重试循环。
type Worker struct {
	repository   Repository
	materializer Materializer
	now          func() time.Time
	interval     time.Duration
	maxAttempts  int
	failure      FailureHandler

	mu      sync.Mutex
	runMu   sync.Mutex
	started bool
	stop    chan context.Context
	done    chan struct{}
}

func NewWorker(repository Repository, materializer Materializer, failure FailureHandler) (*Worker, error) {
	if repository == nil {
		return nil, errors.New("player provision: repository is nil")
	}
	if materializer == nil {
		return nil, errors.New("player provision: materializer is nil")
	}
	if failure == nil {
		return nil, errors.New("player provision: failure handler is nil")
	}
	return &Worker{
		repository:   repository,
		materializer: materializer,
		now:          time.Now,
		interval:     workerInterval,
		maxAttempts:  maxAttempts,
		failure:      failure,
	}, nil
}

func (w *Worker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	w.started = true
	w.stop = make(chan context.Context, 1)
	w.done = make(chan struct{})
	go w.run()
}

func (w *Worker) run() {
	defer close(w.done)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := w.RunOnce(context.Background()); err != nil {
				log.Error("玩家创建 Intent 处理失败: %v", err)
			}
		case stopContext := <-w.stop:
			if err := w.RunOnce(stopContext); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("停止前排空玩家创建 Intent 失败: %v", err)
			}
			return
		}
	}
}

// Stop 停止后续扫描；待处理意图保留在持久化存储中，供下次进程
// 启动时继续处理。上下文限制等待当前扫描结束的时间。
func (w *Worker) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("player provision: context is nil")
	}
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	w.stop <- ctx
	done := w.done
	w.started = false
	w.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunOnce 处理已到重试时间的创建意图，记录失败次数，成功后再标记为已应用。
func (w *Worker) RunOnce(ctx context.Context) error {
	if ctx == nil {
		return errors.New("player provision: context is nil")
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()
	intents, err := w.repository.FindPending(w.now())
	if err != nil {
		return err
	}
	var runErrors []error
	for _, intent := range intents {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(runErrors, err)...)
		}
		if err := w.materializer.Apply(intent); err != nil {
			intent.Attempts++
			intent.LastError = err.Error()
			intent.UpdatedAt = w.now()
			if intent.Attempts >= w.maxAttempts {
				intent.Status = model.StatusFailed
			} else {
				intent.NextAttemptAt = w.now().Add(retryDelay(intent.Attempts))
			}
			if saveErr := w.repository.Save(&intent); saveErr != nil {
				runErrors = append(runErrors, errors.Join(err, saveErr))
			} else {
				runErrors = append(runErrors, err)
			}
			if intent.Status == model.StatusFailed {
				if failureErr := w.failure.HandleProvisionFailure(ctx, intent); failureErr != nil {
					runErrors = append(runErrors, failureErr)
				}
			}
			continue
		}
		// 领域文档全部写入后才更新意图状态；若状态保存失败，下次可幂等重放。
		intent.Status = model.StatusApplied
		intent.LastError = ""
		intent.UpdatedAt = w.now()
		if err := w.repository.Save(&intent); err != nil {
			runErrors = append(runErrors, err)
		}
	}
	return errors.Join(runErrors...)
}

// retryDelay 按失败次数进行指数退避，将单次重试间隔限制在 32 秒以内。
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		return time.Second
	}
	delay := time.Second << min(attempt-1, 5)
	return delay
}

func clonePlayerInfo(info *playermodel.PlayerInfo) *playermodel.PlayerInfo {
	if info == nil {
		return nil
	}
	copyOfInfo := *info
	return &copyOfInfo
}
