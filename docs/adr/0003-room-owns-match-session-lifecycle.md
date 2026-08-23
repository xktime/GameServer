# Room 拥有匹配后对局生命周期

Match 曾同时负责排队、机器人选择、Room 创建、成员关系、Team.RoomId 写入、消息广播和清理，导致匹配决策与对局生命周期无法独立演进。我们决定建立独立 `modules/room`：Match 只形成队伍组合，并通过一方法 `Acceptor.AcceptMatch` 交付带 MatchID 的不可变 Admission；Room 原子提交 Player/Team→Room 权威索引，再把 Team.RoomId 作为可重试的“最新期望值”投影到 Game。

Match 在交付时把真实 Team 从 `queued` 标记为 `settling(MatchID)`。`Accepted` 与 `AlreadyAccepted` 删除请求，`Retryable` 以相同 MatchID 和相同 Admission 重试，`Rejected` 淘汰 Room 指定的真实 Team 并把其余 Team 重新排队；settling 期间取消与普通排队超时均不生效。机器人由 Match 生成纯合成负 ID，不再选择在线真人，因此 ADR 0002 中的机器人读取决策被本 ADR 取代，Match 的 PlayerReader 收窄为在线 Player/Team 两个快照方法。

Room 的 MatchID 在 active Room 期间幂等返回原 Room，关闭后保留 5 分钟 tombstone；Room 关闭会先撤销权威索引并停止自身 actor，再投影清空。投影使用内部版本协调乱序完成，失败只保留最新 RoomID 供重试。匹配结果、操作广播和离线通知均为提交后的 best effort，合成 Robot 永不成为网络收件人；离线通知不会删除成员关系，游戏操作只通过 Room 的 Player→Room 权威映射鉴权。

本切片保持进程内状态，不增加重启恢复或持久化，也不启用当前未被调用的 Timer；`CloseExpired` 仅提供显式的 30 分钟清理入口。定时调度、重连/持久化、全局 actor shutdown、Match 开始/取消通知边界，以及 `S2C_CancelMatch` 与 `S2C_PlayerOffline` 共用 403 的协议迁移，均作为独立后续工作。
