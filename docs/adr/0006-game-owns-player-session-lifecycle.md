# Game 独占 Player Session 生命周期

Game 的 `UserManager` 是 Player Session 的唯一生命周期所有者：登录成功后创建带唯一代次的不可变身份快照，断线时只有与当前代次完全匹配的快照才能关闭会话。持久化 `User` 与连接 `Agent` 不再充当跨模块在线身份；其他模块只消费 Player Session 快照中的 AccountID、PlayerID 和会话 ID。

连接关闭事件必须在广播前捕获快照，避免 Game 清理状态与 Room 等消费者读取状态发生竞态；Player Actor 停止只负责自身持久化，不再清空或关闭连接。这样会增加一个显式会话值和代次校验，但能保证旧连接迟到的断线回调不会误删重登录会话，并把连接、Player Actor 与持久化 User 的职责重新分开。

Gate 在协议分发前验证连接当前持有 Player Session；只有登录与心跳消息允许在未认证状态下进入业务模块。各模块仍以安全解析快照代替强制类型断言，避免内部误路由演变为 handler panic。
