# ADR 0001：由 Rank 定义在线玩家读取接口

- 状态：已接受
- 日期：2026-08-22

## 背景

Rank 当前通过 Game 暴露的具体 `UserManager` 获取可变的 `Player` 对象。这让 Rank 依赖 Game 的内部模型、Actor 所有权和管理器生命周期，也使独立测试必须启动更多基础设施。

本阶段只需要保持现有语义：排行榜仅识别当前在线的 Player，不回退查询 MongoDB 中的离线角色。

## 决策

由消费方 Rank 在 `modules/rank/playerread` 包中定义最小读取接口：

```go
type PlayerReader interface {
	FindOnline(playerID int64) (PlayerSnapshot, bool)
}
```

`PlayerSnapshot` 只包含 Rank 当前需要的玩家名称、头像地址和等级，并按值返回。`false` 统一表示该 Player 当前不在线或底层无法提供在线快照。

Game 提供生产适配器，通过现有在线玩家入口读取 Player 并立即复制快照；Rank 测试使用内存假实现。该适配器是兼容性过渡，只阻止可变 Player 指针继续泄漏到 Rank，并不宣称已经解决旧入口自身的 Actor 并发所有权问题。

依赖在模块启动时显式装配：`RankExternal.InitExternal` 创建 Game 适配器并调用 RankManager 的注册入口；`GetRankManager` 不再创建实例。注册尚未开始时调用 `GetRankManager` 会立即失败，注册进行中则等待结果；注册成功后所有调用返回同一实例。重复注册立即失败；注册过程一旦失败便进入不可重试的终态，当前及后续等待者都会收到同一个失败。

## 结果

- Rank 不再接触 Game 的 `Player`、Actor、Agent 或具体 Manager。
- 现有“仅在线玩家参与读取”的行为保持不变。
- 接口由使用者拥有，后续 Game 内部重构不会直接扩散到 Rank。
- 旧 `GetPlayer` 的 Actor 所有权风险仍留在 Game 内，后续应在 Game 内部单独收口。
- Rank 依赖当前启动顺序中 Game 先完成 `InitExternal`，再由 Rank 开始注册；顺序被破坏时会显式暴露错误。注册开始后，赛季回调等并发读取者可以等待注册结果。
- 两个只检查在线状态的调用点会多读取三个小字段；本阶段接受这点重复，以换取单一且更深的接口。
- `bool` 暂不区分离线与底层故障，因为现有调用也把二者都表现为“无在线 Player”。

## 未采用方案

- 分成 `IsOnline` 与 `RankDisplay` 两个方法：会复制在线语义，并扩大接口和测试矩阵。
- 使用投影、批量或通用查询对象：当前只有三个固定调用点，复杂度超出已确认需求。
- 由 Game 定义接口或继续暴露 `UserManager`：依赖方向仍由提供方模型主导，不能形成稳定边界。
