package actor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// QuarantineTarget 标识一个系统实例中某个 Definition 的隔离代次，
// 不持有 Actor 或 ActorRef。只能由 Definition.QuarantineTarget 取得。
type QuarantineTarget[ID comparable] struct {
	systemID   string
	scopeName  string
	definition reflect.Type
	id         ID
	generation uint64
}

func (t QuarantineTarget[ID]) SystemID() string   { return t.systemID }
func (t QuarantineTarget[ID]) ID() ID             { return t.id }
func (t QuarantineTarget[ID]) Generation() uint64 { return t.generation }

// QuarantineTarget 读取当前隔离身份，不创建实例或返回领域对象。
// 不存在时返回 ErrActorNotFound；初始化中及其他非隔离状态返回 ErrActorNotQuarantined。
func (d *Definition[T, ID]) QuarantineTarget(ctx context.Context, id ID) (QuarantineTarget[ID], error) {
	var zero QuarantineTarget[ID]
	if ctx == nil {
		return zero, fmt.Errorf("actor: context is nil")
	}
	d.scope.mu.Lock()
	defer d.scope.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	entry := d.scope.entries[actorKey{definition: d.key, id: id}]
	if entry == nil {
		return zero, ErrActorNotFound
	}
	if entry.cell == nil || !entry.cell.isQuarantined() {
		return zero, ErrActorNotQuarantined
	}
	return QuarantineTarget[ID]{
		systemID:   d.scope.system.instanceID,
		scopeName:  d.scope.name,
		definition: d.key,
		id:         id,
		generation: entry.generation,
	}, nil
}

// ForceStopQuarantined 只处置目标绑定的隔离代次，跳过 OnStop。
// 目标已移除且未换代时幂等成功；跨系统、Definition 或换代目标返回
// ErrStaleQuarantineTarget。成功表示实例已移除；受理后取消只结束等待，
// 调用方可使用同一目标重试。关联清理和放弃未保存状态的决策由所属领域负责。
func (d *Definition[T, ID]) ForceStopQuarantined(ctx context.Context, target QuarantineTarget[ID]) error {
	if ctx == nil {
		return fmt.Errorf("actor: context is nil")
	}
	d.scope.mu.Lock()
	if err := ctx.Err(); err != nil {
		d.scope.mu.Unlock()
		return err
	}
	key := actorKey{definition: d.key, id: target.id}
	if target.systemID != d.scope.system.instanceID || target.scopeName != d.scope.name ||
		target.definition != d.key || target.generation == 0 ||
		d.scope.generations[key] != target.generation {
		d.scope.mu.Unlock()
		return ErrStaleQuarantineTarget
	}
	entry := d.scope.entries[key]
	if entry == nil {
		d.scope.mu.Unlock()
		return nil
	}
	// 持有注册表锁直到 Cell 接受处置，身份校验与状态迁移期间不能换代。
	attempt, err := entry.cell.requestQuarantinedStop(ctx)
	d.scope.mu.Unlock()
	if err != nil {
		return err
	}
	return waitStop(ctx, attempt)
}

// RebuildQuarantined 供持有单例句柄的领域层显式重建隔离实例。
// cleanup 成功后才放弃旧状态；previous 标识尚未替换的句柄，健康的原实例不会被重建。
// 新代次已创建但领域初始化失败时，重试返回该代次，供调用方继续初始化再发布。
func (d *Definition[T, ID]) RebuildQuarantined(ctx context.Context, id ID, previous ActorRef, cleanup func(context.Context) error) (T, error) {
	var zero T
	if previous.scope != d.scope || previous.key != (actorKey{definition: d.key, id: id}) {
		return zero, ErrStaleActorRef
	}
	target, err := d.QuarantineTarget(ctx, id)
	switch {
	case err == nil:
		if cleanup != nil {
			if err := cleanup(ctx); err != nil {
				return zero, err
			}
		}
		if err := d.ForceStopQuarantined(ctx, target); err != nil {
			return zero, err
		}
	case errors.Is(err, ErrActorNotFound):
	case errors.Is(err, ErrActorNotQuarantined):
		instance, lookupErr := d.Lookup(ctx, id)
		if lookupErr != nil {
			return zero, lookupErr
		}
		if instance.ActorBase().Ref() == previous {
			return zero, err
		}
		return instance, nil
	default:
		return zero, err
	}
	return d.GetOrCreate(ctx, id)
}
