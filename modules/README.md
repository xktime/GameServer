# 模块与 Actor 管理

模块间通过 Registry、Manager 或消费方定义的领域接口协作。公开接口接收领域标识、请求和上下文，返回值快照、领域结果或错误。运行中的 Actor、`BaseActor` 和 `ActorRef` 留在所属包内。

## 运行时的职责

```text
ActorSystem
  └─ Scope：模块的创建与关闭边界
       └─ Definition[T, ID]：工厂、身份、初始化与实例查找
            └─ Cell：邮箱、执行循环与生命周期
```

`common/base/actor` 按职责拆分：`definition.go` 管理注册及初始化，`ref.go` 定义调用语义，`mailbox.go` 管理接收及执行，`lifecycle.go` 管理停止和隔离，`scope.go` 与 `actor_system.go` 管理关闭边界，`context.go` 传递同步调用链，`metrics.go` 汇总运行指标。

Entry 只持有初始化结果及 Cell 引用。运行、停止中、保存失败、强制停止、已停止和 panic 隔离均由 Cell 的唯一状态管理。同一 Cell 的 handler 和停止钩子始终在同一执行循环中串行运行。

## 注册和领域入口

Actor 类型嵌入 `actor.BaseActor`，满足 `actor.Actor`。每个定义显式声明邮箱容量，运行时不认识 Player、Team 等业务分组。

```go
type playerActor struct {
    actor.BaseActor
    // 运行时状态和依赖
}

type Registry struct {
    definition *actor.Definition[*playerActor, int64]
}

func newRegistry(scope *actor.Scope) (*Registry, error) {
    definition, err := actor.Define(scope,
        func(context.Context, int64) (*playerActor, error) {
            return &playerActor{}, nil
        }, actor.DefinitionOptions{QueueCapacity: 256})
    if err != nil {
        return nil, err
    }
    return &Registry{definition: definition}, nil
}
```

生产定义目前为 Player、Team、Room 各 256；User、Connect、Match、Rank 各 1024。Team Registry 另有一个容量为 256 的广播派发邮箱。容量为零时运行时默认采用 256，负数被拒绝。

公开 Manager 使用命名的私有 Actor 字段，而不匿名嵌入 Actor。这样调用者只能使用领域方法，无法通过提升的方法取得 Ref 或绕过邮箱修改状态。RoomManager 的跨房间索引继续由其互斥锁管理，Room 实例为私有类型。

User、Connect、Match、Rank 单例 Manager 保留 Definition，并在恢复成功后同步替换内部实例；其他模块持有的 Manager 句柄继续有效。User 恢复沿用共享的登录会话状态及启动时设置的 Room 下线通知器。

LoginManager 直接持有认证池和登录尝试跟踪器，由两者保证并发安全。TeamManager 直接组合 Player、Team Registry；读取和投影进入对应领域实例。广播先在已有 Team 的邮箱内预留该代次的在途计数并复制成员，再向独立的有界派发邮箱提交；返回成功只表示派发已接收，调用方取消不撤销已接收的广播。发送按派发入队顺序执行，单个成员的错误或 panic 不阻断后续成员，Player 邮箱拥塞不占用 Team 状态邮箱。Team 停止封闭该代次的预留并等待其在途任务，覆盖复制成员到派发接收之间的间隙；没有在途广播的 Team 不等待其他队伍。game 模块先关闭并排空派发器，再停止包含 Player 的 Scope，确保已接收广播仍能访问接收方。

## Query、Command 和通知

| API | 用途 | 上下文与结果 |
| --- | --- | --- |
| `Query` | 读取快照、数量、排行榜 | 调用方可按截止时间结束等待；超时返回上下文错误，已接收的查询可能仍执行 |
| `Command` | 修改状态、提交登录、同步派发等需要最终结果的操作 | 上下文可取消接收前的等待；一经接收，保留上下文值并解除取消，等待 handler 的终态结果 |
| `Tell` | 明确允许异步执行的通知 | 等待接收，不等待领域执行结果；已接收任务解除调用方取消 |
| `TryTell` | 可丢弃或由调用方重试的通知 | 立即尝试接收，队列满返回 `ErrQueueFull` |

接收点是 Cell 在同一把锁内检查状态并提交队列的位置。停止先关闭入口并唤醒等待接收的调用者，然后排空已接收任务。强制停止明确拒绝排队任务并跳过尚未开始的停止钩子，但仍须等待当前 handler 返回。

`Command` 接收后的等待没有调用方超时退路。因此 handler 使用的数据库、网络等 I/O 必须自行有界；不能把接收超时当作“写入失败”。`Tell` 返回 nil 只表示任务已接收，handler 的错误由运行时记录，不应据此向客户端回复领域操作成功。

User、Rank 初始化仍使用 `Command` 串行更新状态，但名字预加载、排行榜读取及迁移 I/O 显式使用原始初始化上下文，保留启动取消及截止时间。

有缓存写入的“读取”仍使用 `Command`，例如名字查询的最终缓存更新和刷新挑战列表。排行榜的纯读取使用 `Query`，不会隐式创建空榜。

User 名字缓存只保存已占用或已预留的名字。名字检查先在 User 邮箱内读取缓存，未命中时在邮箱外查询仓库，再回到原代次邮箱复核最新缓存并登记已占用名字。数据库读取不阻塞其他登录和会话操作；原代次已停止、隔离或被替换时返回失败。未命中的名字不加入缓存，后续检查重新查询仓库，避免任意输入持续扩大负缓存，也能及时发现新占用的名字。

## 上下文与同步调用链

handler 内调用另一个 Registry、Manager 或适配器时，传递收到的 `actor.Context` 或由其派生的标准 context。Match/Rank 的 PlayerReader、Room 的匹配接收与 Team 投影接口都传递上下文。

Match/Rank 的 PlayerReader 返回 `(snapshot, found, error)`。确实离线、无队伍或 Rank 所需资料不完整时返回 `false, nil`；取消、隔离、同步调用环及仓库错误保留到调用边界，查询失败不会修改匹配队列或排行榜状态。`Lookup` 对已取消的请求优先返回上下文错误。

```go
_, err := actor.Command(ctx, instance.Ref(), func(execution actor.Context) (struct{}, error) {
    return struct{}{}, teams.Join(execution, teamID, playerID)
})
```

同一同步调用链再次进入任一已占用 Actor 时返回 `ErrCallCycle`，包括通过 `context.WithTimeout` 等派生的上下文。Actor 内复用自己的逻辑应直接调用私有普通方法。不要用 `context.Background()` 替换同步调用链，也不要在 handler 返回后或另一 goroutine 中继续复用执行上下文。异步通知开始新的同步调用链。

仓库操作显式接收上下文，Mongo 的 `FindOneContext`、`FindOneByIDContext`、`FindAllContext`、`SaveContext`、`DeleteByIDContext` 和 `SaveIfAbsentContext` 保留父取消及更早的截止时间。单次查询/写入上限仍为 5 秒，多条查询为 10 秒。创建意图的 Materializer 与默认登录处理器同样传递调用方上下文。Mongo 未初始化时，写入返回错误，不再以空结果表示成功。

## 停止、保存失败与重试

`Registry.Stop(ctx, id)` 委托 `Definition.Stop(ctx, id)`，不经过只返回可运行实例的 `Lookup`。

1. 关闭入口并排空已接收任务。
2. 同一执行循环调用 `OnStop(ctx)`，由领域层保存或删除持久化数据。
3. 成功后移除实例，允许相同 ID 再次创建。
4. 失败后保留原实例与执行循环，拒绝业务请求及同 ID 替换；下一次显式 `Stop` 使用新的上下文重试。

并发 `Stop` 共享正在执行的同一次保存及其结果。调用方等待超时不移除实例，也不启动并行保存。`ForceStop` 会放弃未持久化状态，只用于明确拥有未提交实例的回滚等场景。

需要在当前任务内决定停止时，使用 `execution.StopSelf(stopCtx)` 关闭入口，并把返回的 `StopResult` 带出任务后调用 `Wait(ctx)`。`Wait` 始终观察原尝试，不会因保存已经失败而意外启动重试。Player 下线和 Team 最后一名成员离开使用此路径；Team 已接收的后续加入请求也会在执行时检查删除状态，不能复活正在删除的实例。

未提交 Team 的登录回滚封闭该代次的广播预留，再使用 `execution.ForceStopSelf()` 发起一次强停，在任务外等待返回的 `StopResult` 和在途广播，跳过持久化。重试回滚仍等待原代次，不修改同 ID 的新实例。隔离 Team 的恢复先排空其在途广播，再清理关联并放弃隔离状态。

连续 handler panic 达到阈值后，实例持续隔离，`Lookup`、`GetOrCreate` 和普通 `Stop` 返回 `ErrActorQuarantined`，不调用 `OnStop`，也不按时间自动重建。Scope／ActorSystem 的普通停止汇总隔离错误，并继续停止其他实例。

所属模块通过 `definition.QuarantineTarget(ctx, id)` 取得不含 Actor 引用的隔离目标；`SystemID()`、`ID()` 和 `Generation()` 可用于记录身份。显式恢复指令完成该代次的领域关联清理后，调用 `definition.ForceStopQuarantined(ctx, target)` 放弃未保存状态。目标绑定系统实例、Scope、Definition、ID 和 generation，跨系统或换代目标返回 `ErrStaleQuarantineTarget`；同一目标已移除且未换代时幂等成功。并发处置共享完成结果，成功表示实例已移除，此后可正常创建同 ID 的新代次。

处置受理前已取消的上下文不会发起强停；受理后超时只结束等待，返回可同时匹配 `ErrActorStopping` 和上下文错误的结果，处置继续，可用同一目标重试。隔离仅保留在当前 ActorSystem 内，新系统使用不同实例标识。

领域入口使用显式 `Recover`。User、Connect、Match 和 Player/Team Registry 要求调用方提供关联清理回调，先解除会话、Room、Team 或未完成匹配的关联，清理失败时保留隔离实例。Player/Team 处置后不立即激活空实例，下一次登录仍从正常的准备和提交入口恢复。Room 自行清理索引及 Team 投影，投影失败可重试；Rank 从最后持久化快照恢复。单例初始化失败不发布新实例，后续恢复调用可继续初始化同一代次。

恢复入口使用支持上下文取消的锁；等待锁时即可按截止时间返回，不执行清理。Player、Team、Room 按实体 ID 串行，不同 ID 的恢复互不等待；User、Connect、Match、Rank 在各自单例内串行。锁条目在无持有者和等待者后移除。

| 错误 | 含义与处理 |
| --- | --- |
| `ErrActorNotFound` | Definition 中不存在实例；可由领域入口决定是否加载离线数据或创建 |
| `ErrActorStopping` | 正在停止，拒绝新业务；等待停止超时还可匹配原上下文错误 |
| `ErrStopFailed` | 停止钩子失败；可同时通过 `errors.Is` 匹配底层仓库错误，显式 Stop 重试 |
| `ErrActorStopped` | 某个 Ref 对应的实例已经移除 |
| `ErrStaleActorRef` | ID 已重建，旧代次的 Ref 不可使用 |
| `ErrActorQuarantined` | 连续 handler panic 达到阈值后持续隔离，等待所属模块显式处置 |
| `ErrActorNotQuarantined` | 读取隔离目标时，该实例尚在初始化或处于其他非隔离状态 |
| `ErrStaleQuarantineTarget` | 隔离目标所属系统、Scope、Definition 或 generation 不匹配，拒绝处置 |
| `ErrScopeStopped` / `ErrSystemStopped` | 关闭边界已经禁止创建 |

## 数据与激活边界

`player.Player`、`team.Team` 是持久化数据，运行中的类型分别为私有的 `playerActor`、`teamActor`。Mongo 集合由 Go 类型名决定，因此持久化类型名及既有 BSON 字段保持不变，避免把数据写到新集合。

`Team.Registry.Snapshot` 在已有实例上读取快照；确实不存在实例时直接从仓库读取离线快照，既不激活实例，也不安排停止保存。激活通过登录准备入口完成。广播只向已有 Team 提交任务。实例正在停止、保存失败或被隔离时，不回退读取可能过时的持久化数据。

Player 的加载、登录准备、提交与回滚保留独立领域步骤。普通请求在邮箱内检查是否已经提交登录，未提交时返回 `ErrPlayerNotReady`。Session Takeover 保留在线 Player 状态及 Team、Room 归属，遵循根目录 `CONTEXT.md` 的领域约束。

Player 的离线资料、名字查询及名字预加载都通过 Player Registry 的同一个 Repository。`StoredSnapshot` 只读取并复制持久化资料，不激活 Actor。Player 和 Team 的 `NewRegistryWithRepository` 可替换默认 Mongo 实现，单测使用显式注入的仓库。UserManager 不直接查询 Player 集合。

Rank Actor 在邮箱内导出独立的 `rankState` 持久化快照，包含各榜单、赛季及挑战码副本。仓库只接收快照，按榜单全部成功、元数据、赛季、过期榜清理的顺序保存。

## 模块关闭与验证

模块先停止定时任务、认证池或创建意图 Worker，再停止 Scope。定时任务停止后禁止再次调度，等待当前回调也受关闭预算限制。实现 `module.ShutdownModule` 的模块通过 `Shutdown() error` 返回关闭结果，运行时汇总失败并继续按逆序关闭其他模块；服务入口保留这些错误以及 HTTP 关闭错误。

RoomManager 关闭时同时补交未完成的 Team 投影，包括已移除房间的清理。room 模块先用 5 秒预算停止维护任务及管理器，清理出现普通错误或预算耗尽时，在 game 停止保存 Team 前用新的 5 秒预算重试；随后用独立的 5 秒预算停止 Scope。持续清理失败会传回服务入口。认证池等待超时后取消在途认证，排队认证以失败完成；login 模块在停止 Scope 前用新的 30 秒预算继续等待工作协程退出。

Room 的关闭记录从首次关闭起保留 5 分钟，供重复关闭返回幂等结果。维护任务仅在 Actor 已停止且相关 Team 的最新投影已完成时回收到期记录；未完成的停止、隔离处置或投影继续保留供重试。

game 创建意图 Worker 的停止超时会取消当前扫描，后续 `Stop` 仍等待同一工作协程退出，已停止的 Worker 不再次启动。game 模块首次用 5 秒预算等待，失败后使用新的 30 秒预算继续等待；之后分别用独立的 5 秒预算排空广播和停止 Scope，失败均传回服务入口。未完成的意图保留供后续重放。

进程的最终 `ActorSystem.Stop` 使用新的 30 秒上下文，再次处理尚未完成或保存失败的实例。若 `Scope.Stop` 加入了旧的停止尝试，随后观察到旧上下文取消后的保存失败，它会在本次剩余预算内重试一次。普通并发 `ActorRef.Stop` 和原 `StopResult` 的结果保持不变。截止时间只限制等待及支持取消的 I/O，无法强制终止任意 Go handler。

修改运行时或领域入口后执行：

```powershell
go test ./...
go test -race ./...
```

回归测试覆盖最终关闭的保存重试、并发停止共享结果、停止截止时间、房间投影补交与 Team 保存顺序、模块关闭错误传播、各代 Team 广播与停止的交错、广播排空与接收方关闭顺序、名字查询与登录并发及缓存复核、认证和创建意图 Worker 的取消及退出等待、隔离清理失败重试及单例重新绑定、初始化取消与截止时间、取消后的 Command 归属、未提交登录的访问限制、Player 查询仓库注入、跨模块错误传播、Rank 存档副本独立性、Team 离线查询与排队加入边界，以及 Player/Team 的 BSON 兼容性。排行榜 Mongo 集成测试可通过 `MONGODB_URI` 指定实例；设置 `MONGODB_REQUIRED=1` 可将实例不可用视为失败。
