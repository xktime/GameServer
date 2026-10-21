# GameServer 领域模型

本文档定义 GameServer 的统一领域语言。

## 领域语言

**User**:
一个平台身份在单个游戏服务器中的注册身份，与且仅与一个 Player 关联。
_避免使用_: Account、Player、Session

**Player**:
由一个 User 控制的持久化游戏角色。
_避免使用_: User、Agent、Session

**Player Session**:
Player 在一次在线期间的临时存在。
_避免使用_: Player、User、Agent

**Login Attempt**:
连接发起的一次建立 Player Session 的尝试，具有服务器登记的先后顺序。连接断开、同一连接出现更新尝试，或同一 User 已有登记更晚且成功的尝试时，未完成的旧尝试失效，不能建立 Player Session 或返回登录成功。
_避免使用_: Player Session、Match Attempt、登录结果

**Session Takeover（会话接管）**:
同一 User 的新 Player Session 在登录准备成功且 Login Attempt 仍有效时替代旧会话，保留 Player 的当前游戏状态及 Team、Room 归属。准备失败或尝试失效不替代旧会话，旧连接的关闭不代表 Player 下线。
_避免使用_: Player 下线、重新建角、断线重登

**Team**:
一组共同参与组队行为的 Player；进入匹配时作为不可拆分的整体，并共享 Room 归属。
_避免使用_: Group、Party、Room

**Match Attempt**:
一次把已匹配 Team 交付为 Room 的幂等尝试，由唯一 MatchID 标识。
_避免使用_: Match Result、Room Request

**Room**:
由 Match Attempt 创建的临时对局容器，是 Player 与 Team 当前 Room 归属的权威来源。
_避免使用_: Match、Lobby、Team

**Robot**:
Match 为补齐对局人数创建的合成参与者，不对应任何 User、Player、Player Session 或 Team。
_避免使用_: Online Player、Bot Player

**Challenge Code**:
用于发起对某个 Player 的挑战、具有固定到期时间的编号；正常重启不改变未过期编号的有效性。
_避免使用_: MatchID、Player ID、Session ID

**Leaderboard（排行榜）**:
按排行类型及适用的赛季组织的 Player 成绩集合；前 1000 名仅是列表展示范围，不代表成绩保留范围。榜外 Player 保留成绩，但视为未上榜，不提供榜外的具体名次。
_避免使用_: 榜单缓存、前 1000 名列表
