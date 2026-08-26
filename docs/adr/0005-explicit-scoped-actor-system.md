# 显式 Scope ActorSystem 取代进程级全局 Actor

GameServer 不再使用进程级 `TaskHandler` 注册表和反射式 `SendTask`。组合根显式创建一个 `ActorSystem`，每个业务模块拥有独立 `Scope`，每种 Actor 通过唯一 `Definition` 工厂创建，并只通过有代际校验的 `ActorRef` 接收类型化 `Call`、`Tell` 或 `TryTell`；这是一次性迁移，不保留新旧运行时兼容桥，以免身份、停止和持久化语义分裂。

Actor 的有界 FIFO 邮箱负责串行化可变状态；同步调用环路会被拒绝，异步 Tell 开启新的调用链。模块销毁时先停止周期任务，再由 Scope 拒绝新任务、排空已接纳任务并调用 Actor 自己的 `OnStop` 持久化；组合根最后停止整个系统，强制停止只用于显式放弃队列和持久化的故障恢复。连续 panic 会隔离当前代实例，冷却后才能由 Definition 建立新代。

Definition factory 只建立结构完整的内存对象，不执行数据库加载等业务初始化；User 与 Rank 在创建后通过 Actor command 初始化，并在模块给定的启动时间预算内复用同一实例退避重试，避免重复注册 factory 或在失败后遗失实例。

跨 Actor 和跨模块边界不再返回可变 Actor 指针：Game 内使用 Player/Team Registry 定位实例，Match、Rank、Room 和协议处理器只依赖消费方接口和值快照。代价是内部 API 的集中破坏性迁移和更严格的启动顺序，但 Actor 所有权、失败状态、关闭顺序及数据竞争边界现在都可被测试和观测；协议与 MongoDB 文档字段保持兼容。

本 ADR 取代 ADR 0001、0002、0003、0004 中“Game 内部 Actor 所有权与全局 Actor shutdown 仍待后续处理”的过渡性说明；这些 ADR 的消费方接口、Room 生命周期和模块自有周期维护决策继续有效，旧的进程级 Schedule API 仍不在本次迁移范围内。
