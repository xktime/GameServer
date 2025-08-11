# Actor Agent Generator

## 概述

Actor Agent Generator 是一个Go代码生成工具，用于自动生成Actor模式的代理代码。它能够扫描Go源代码，识别包含`actor_manager.ActorMessageHandler`的结构体，并生成相应的Actor代理代码。

## 功能特性

- **自动检测：** 自动扫描目录，识别符合条件的结构体
- **智能分类：** 根据结构体名称自动判断生成规则
- **类型安全：** 保持原始方法的参数类型和返回值类型
- **依赖管理：** 自动检测并导入必要的包
- **批量生成：** 支持一次生成多个结构体的代理代码

## 生成规则

### Manager类型结构体
- **识别条件：** 结构体名称以`Manager`结尾
- **生成模式：** 代理类模式，提供`DirectCaller`直接调用
- **使用场景：** 需要频繁调用的管理器类

### 非Manager类型结构体
- **识别条件：** 结构体名称不以`Manager`结尾
- **生成模式：** 函数模式，只能通过Actor队列调用
- **使用场景：** 实体对象，如Player、Monster等

## 使用方法

### 命令行参数

```bash
go run . -source <源目录路径>
```

- `-source`: 指定包含Go源代码的目录路径

### 使用示例

```bash
# 生成game模块的Actor代理代码
cd tools/actor_agent_generator
go run . -source ../../modules/game/internal/managers

# 生成login模块的Actor代理代码
go run . -source ../../modules/login/internal/managers
```

### 批量生成脚本

使用提供的批处理脚本：

```bash
# Windows
generate_from_file.bat

# 或手动指定目录
go run . -source <目录路径>
```

## 输出文件

### 文件命名规则
- 输出文件：`{结构体名小写}_actor.go`
- 位置：与源文件相同的目录

### 生成内容示例

#### Manager类型输出
```go
type UserManagerActorProxy struct {
    DirectCaller *UserManager
}

func GetUserManager() *UserManagerActorProxy
func (*UserManagerActorProxy) DoLogin(agent gate.Agent, openId string, serverId int32)
```

#### 非Manager类型输出
```go
func Print(PlayerId int64)
func SendToClient(PlayerId int64, message proto.Message)
```

## 技术实现

### 代码解析
- 使用Go标准库的`go/ast`包解析源代码
- 支持Go 1.16+语法特性
- 自动处理导入和包依赖

### 模板系统
- 使用Go标准库的`text/template`包
- 支持自定义模板函数
- 根据结构体类型选择不同模板

### 类型处理
- 自动识别方法参数和返回值类型
- 支持复杂类型（指针、数组、接口等）
- 生成类型安全的代码

## 配置选项

### 自动检测配置
- 扫描`.go`文件（排除`*_actor.go`）
- 查找包含`actor_manager.ActorMessageHandler`字段的结构体
- 验证结构体是否有对应的方法

### 输出配置
- 自动创建输出目录
- 保持包名和导入路径
- 生成完整的Go代码文件

## 最佳实践

### 1. 目录结构
```
modules/
  game/
    internal/
      managers/
        user_manager.go      # 原始结构体
        user_manager_actor.go # 生成的代理代码
```

### 2. 命名规范
- Manager结构体：`UserManager`、`PlayerManager`
- 实体结构体：`Player`、`Monster`、`Item`

### 3. 字段要求
```go
type UserManager struct {
    actor_manager.ActorMessageHandler  // 必需字段
    // ... 其他字段
}
```

### 4. 方法定义
```go
func (m *UserManager) DoLogin(agent gate.Agent, openId string) {
    // 方法实现
}
```

## 故障排除

### 常见错误

1. **"未找到包含 ActorMessageHandler 的结构体"**
   - 检查结构体是否包含正确的字段
   - 确认字段类型为`actor_manager.ActorMessageHandler`

2. **"未找到结构体的方法"**
   - 确认结构体有receiver方法
   - 检查方法语法是否正确

3. **"创建输出目录失败"**
   - 检查目录权限
   - 确认路径存在且可写

4. **"生成代码失败"**
   - 检查原始代码语法
   - 查看详细的错误信息

### 调试技巧

1. **启用详细日志**
   - 检查控制台输出
   - 确认扫描到的结构体和方法

2. **验证输出文件**
   - 检查生成的`*_actor.go`文件
   - 确认代码语法正确

3. **检查依赖**
   - 确认`actor_manager`包可用
   - 验证导入路径正确

## 扩展开发

### 自定义模板
可以修改`generator.go`中的模板来定制生成代码的格式和内容。

### 添加新类型
在`AutoDetectStructs`函数中添加新的结构体识别逻辑。

### 支持新特性
扩展`MethodInfo`结构体来支持更多的方法特性。

## 版本要求

- Go 1.16+
- 支持Go modules
- 兼容Windows、Linux、macOS

## 许可证

本项目遵循项目主许可证。

