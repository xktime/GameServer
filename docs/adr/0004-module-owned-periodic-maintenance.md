# 模块拥有可停止的周期维护

ADR 0003 把定时维护留作后续工作；现在决定由 Match 与 Room 各自在模块生命周期内拥有一个注入式 Scheduler 和可停止 Job，而不是把领域维护注册到进程级全局调度表。Job 串行执行回调并在模块销毁时先停止，因此不会重叠执行，也不会在所属模块关闭后继续访问其状态。

Match 每 10 秒执行已有匹配周期，所有时间判断使用注入 Clock；Room 每 10 秒依次关闭超时 Room、重试最新 Team→Room 投影并清理到期 MatchID tombstone。这个切片不迁移旧的全局 Schedule API、不改变全局 actor shutdown，也不引入状态持久化。
