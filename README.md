# GameServer - 游戏服务器

一个基于Go语言开发的高性能游戏服务器，采用Actor模式进行并发管理，支持多种游戏功能模块。

## 🚀 项目特性

- **高性能架构**: 基于Actor模式的并发处理，支持高并发连接
- **模块化设计**: 清晰的模块分离，易于扩展和维护
- **多协议支持**: 支持WebSocket、TCP等多种网络协议
- **数据持久化**: 集成MongoDB进行数据存储
- **实时通信**: 支持实时消息推送和房间管理
- **充值系统**: 完整的游戏充值功能
- **排行榜系统**: 实时排行榜功能
- **匹配系统**: 游戏匹配和房间管理
- **HTTP API**: 提供GM管理接口

## 📁 项目结构

```
GameServer/
├── common/                 # 公共组件
│   ├── base/actor/        # Actor模式基础组件
│   ├── config/            # 配置管理
│   ├── db/mongodb/        # MongoDB数据库操作
│   ├── event_dispatcher/  # 事件分发器
│   ├── models/            # 数据模型
│   ├── msg/               # 消息定义和协议
│   ├── schedule/          # 定时任务
│   └── utils/             # 工具函数
├── core/                  # 核心框架
│   ├── chanrpc/           # 通道RPC通信
│   ├── cluster/           # 集群管理
│   ├── gate/              # 网关模块
│   ├── log/               # 日志系统
│   ├── module/            # 模块管理
│   ├── network/           # 网络通信
│   ├── server/            # 服务器核心
│   └── timer/             # 定时器
├── modules/               # 业务模块
│   ├── game/              # 游戏核心模块
│   ├── login/             # 登录模块
│   ├── match/             # 匹配模块
│   └── rank/              # 排行榜模块
├── gate/                  # 网关路由
├── http/                  # HTTP服务器
├── conf/                  # 配置文件
├── tools/                 # 开发工具
└── test/                  # 测试文件
```

## 🏗️ 核心架构

### Actor模式
项目采用Actor模式进行并发管理，每个业务实体都是一个Actor，通过消息队列进行通信，确保数据一致性和线程安全。

### 模块化设计
- **game模块**: 游戏核心逻辑，包括用户管理、玩家信息、充值系统
- **login模块**: 用户登录认证，支持多种登录方式
- **match模块**: 游戏匹配和房间管理
- **rank模块**: 排行榜系统
- **gate模块**: 网络网关，处理客户端连接和消息路由

### 通信机制
- **ChanRPC**: 模块间异步通信
- **Event Dispatcher**: 事件分发系统
- **Protobuf**: 消息序列化协议

## 🛠️ 技术栈

- **语言**: Go 1.24.5
- **数据库**: MongoDB
- **缓存**: Redis
- **网络**: WebSocket, TCP
- **序列化**: Protocol Buffers
- **HTTP框架**: Gin
- **容器化**: Docker

## 🚀 快速开始

### 环境要求
- Go 1.24.5+
- MongoDB
- Redis
- Docker (可选)

### 安装依赖
```bash
go mod download
```

### 配置设置
1. 复制配置文件到 `conf/` 目录
2. 修改 `conf/server.json` 中的数据库连接信息
3. 根据需要调整其他配置参数

### 启动服务

#### 使用Docker (推荐)
```bash
# 启动Redis和MongoDB
docker network create my-network
docker pull redis:latest
docker run -itd --name redis --network my-network -p 6379:6379 redis

docker pull mongo:latest
docker run -d -p 27017:27017 --network my-network --name mongo mongo

# 构建和运行游戏服务器
docker build -t gameserver:latest .
docker run -p 3653:3653 -p 3563:3563 --network my-network --name gameserver gameserver:latest
```

#### 本地运行
```bash
# 启动MongoDB和Redis服务
# 然后运行游戏服务器
go run main.go
```

### 开发工具

#### Actor代理生成器
项目提供了自动生成Actor代理代码的工具：

```bash
cd tools/actor_agent_generator
go run . -source ../../modules/game/internal/managers
```

#### 配置生成器
```bash
cd tools/config_generator
go run .
```

#### 处理器生成器
```bash
cd tools/handler_generator
go run .
```

## 📊 性能特性

- **高并发**: 基于Actor模式的并发处理
- **内存优化**: 使用布隆过滤器优化名称检查
- **连接池**: MongoDB连接池管理
- **消息队列**: 异步消息处理
- **缓存机制**: 多级缓存策略

## 🔧 配置说明

### 服务器配置 (`conf/server.json`)
```json
{
  "MaxConnNum": 10000,
  "WSAddr": ":3653",
  "TCPAddr": ":3563",
  "HttpPort": 8080,
  "MongoDB": {
    "Host": "mongodb://localhost:27017",
    "Database": "gameserver"
  }
}
```

### 游戏配置
- `GameConst.json`: 游戏常量配置
- `match.json`: 匹配系统配置
- `recharge.json`: 充值系统配置

## 🧪 测试

```bash
# 运行所有测试
go test ./...

# 运行特定模块测试
go test ./modules/game/...

# 运行数据库测试
go test ./test/db_test.go
```

## 📝 API文档

### WebSocket消息协议
所有客户端通信都通过WebSocket进行，使用Protocol Buffers进行消息序列化。

#### 消息格式
WebSocket消息采用二进制格式，结构如下：

```
[1字节] + [4字节] + [4字节] + [消息体]
  ↓         ↓         ↓         ↓
是否回复   回复序号  消息ID    Protobuf消息结构
```

**字段说明：**
- **第1字节**: 是否为回复客户端的消息标识
  - `0`: 普通消息
  - `1`: 回复消息
- **第4字节**: 回复消息序号（仅当第1字节为1时有效）
  - 用于匹配请求和响应
  - 大端序存储
- **第4字节**: 消息ID
  - 标识消息类型
  - 大端序存储
- **消息体**: Protobuf序列化后的消息内容

**示例：**
```
普通消息: [0][0x00,0x00,0x00,0x01][消息体]
回复消息: [1][0x00,0x00,0x00,0x05][0x00,0x00,0x00,0x01][消息体]
```

#### 消息ID定义
消息ID在 `common/msg/message/message_id.pb.go` 中定义，包括：
- 登录相关消息
- 游戏操作消息  
- 匹配系统消息
- 排行榜消息
- 充值系统消息

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

---

**注意**: 这是一个游戏服务器项目，请确保在合法合规的前提下使用。