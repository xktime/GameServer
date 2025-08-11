# Modules 模块说明文档

## 概述

本目录包含了游戏服务器的各个功能模块，每个模块都使用Actor模式进行并发管理。通过`actor_agent_generator`工具可以自动生成Actor代理代码，简化模块间的调用。

## Actor Agent Generator 生成规则

### 1. 自动检测规则

生成器会自动扫描指定目录，查找包含以下特征的结构体：
- 结构体字段中包含 `actor_manager.ActorMessageHandler` 类型
- 结构体有对应的方法（receiver方法）

### 2. 生成规则分类

#### 2.1 Manager类型结构体
**识别条件：** 结构体名称以`Manager`结尾

**生成内容：**
- 生成`{StructName}ActorProxy`代理结构体
- 提供`DirectCaller`字段，用于直接调用（绕过Actor队列）
- 提供`Get{StructName}()`函数获取代理实例
- 所有方法都通过Actor队列异步执行

**使用方式：**
```go
// 通过Actor队列调用（异步）
proxy := GetUserManager()
proxy.DoLogin(agent, openId, serverId)

// 直接调用（同步，绕过Actor队列）
proxy := GetUserManager()
proxy.DirectCaller.DoLogin(agent, openId, serverId)
```

#### 2.2 非Manager类型结构体
**识别条件：** 结构体名称不以`Manager`结尾

**生成内容：**
- 生成独立的函数，每个函数对应一个方法
- 函数名与方法名相同
- 第一个参数为`{StructName}Id int64`
- 只能通过Actor队列调用

**使用方式：**
```go
// 只能通过Actor队列调用
Print(playerId)
SendToClient(playerId, message)
```

### 3. 方法类型处理

#### 3.1 无返回值方法
生成`actor_manager.Send`调用，异步执行

#### 3.2 有返回值方法
生成`actor_manager.RequestFuture`调用，支持同步等待结果

#### 3.3 通用参数方法
如果方法签名为`func(args []interface{})`，生成器会特殊处理，直接传递参数

## 使用示例

### Manager类型示例

```go
// 原始结构体
type UserManager struct {
    actor_manager.ActorMessageHandler
    // ... 其他字段
}

// 生成后的代理
type UserManagerActorProxy struct {
    DirectCaller *UserManager
}

// 获取代理实例
func GetUserManager() *UserManagerActorProxy

// 异步调用
proxy := GetUserManager()
proxy.DoLogin(agent, openId, serverId)

// 同步调用（绕过Actor队列）
proxy := GetUserManager()
proxy.DirectCaller.DoLogin(agent, openId, serverId)
```

### 非Manager类型示例

```go
// 原始结构体
type Player struct {
    actor_manager.ActorMessageHandler
    // ... 其他字段
}

// 生成后的函数
func Print(PlayerId int64)
func SendToClient(PlayerId int64, message proto.Message)

// 使用方式
Print(playerId)
SendToClient(playerId, message)
```

## 生成工具使用

### 自动生成
```bash
cd tools/actor_agent_generator
go run . -source ../../modules/game/internal/managers
```

### 手动生成
```bash
cd tools/actor_agent_generator
go run . -source <源目录路径>
```

## 最佳实践

### 1. 命名规范
- Manager类型结构体必须以`Manager`结尾
- 非Manager类型结构体避免使用`Manager`后缀

### 2. 调用选择
- **性能敏感场景：** 使用`DirectCaller`直接调用
- **并发安全场景：** 使用Actor队列调用
- **数据一致性要求：** 使用Actor队列调用

### 3. 错误处理
- 异步调用无法直接获取错误信息
- 同步调用可以获取返回值，包括错误信息

### 4. 生命周期管理
- Actor实例通过`Get{StructName}()`获取，单例模式
- 避免在多个地方创建Actor实例

## 注意事项

1. **线程安全：** `DirectCaller`直接调用不是线程安全的，需要自行保证
2. **性能考虑：** Actor队列调用有额外开销，频繁调用建议使用`DirectCaller`
3. **依赖管理：** 生成的代码会自动检测并导入必要的包
4. **方法签名：** 生成器会保持原始方法的参数类型和返回值类型

## 故障排除

### 常见问题

1. **找不到结构体：** 检查结构体是否包含`actor_manager.ActorMessageHandler`字段
2. **方法未生成：** 确认结构体有对应的receiver方法
3. **导入失败：** 检查生成的代码中的包路径是否正确
4. **编译错误：** 确认原始结构体代码没有语法错误

### 调试技巧

1. 检查生成的`*_actor.go`文件内容
2. 确认结构体字段类型正确
3. 验证包名和导入路径
4. 查看生成器的日志输出
