# Handler生成器

这个工具用于根据proto文件自动生成对应的handler文件，并自动更新相关的注册文件。

## 功能特性

- 自动扫描proto文件中的C2S消息
- 根据proto文件路径自动判断所属模块（game/login）
- 将handler文件生成到对应模块的handlers目录
- 支持递归扫描子目录中的proto文件
- 避免重复生成已存在的文件
- **自动生成pb.go文件**：修正了protoc命令执行，确保正确生成Go代码
- **智能路径处理**：正确处理子目录中的proto文件导入路径
- **自动注册功能**：生成handler的同时自动更新以下文件：
  - `modules/{module}/internal/handler.go` - 注册消息处理器到ChanRPC
  - `gate/router.go` - 设置消息路由到对应模块
  - `common/msg/msg.go` - 注册消息到Processor
- **自动删除功能**：当proto文件被删除时，自动删除对应的handler文件和注册信息

## 使用方法

### 命令行参数

- `-proto`: proto文件目录路径
- `-output`: 输出目录路径（可选，默认为`../../common/msg/message/handlers`）
- `-modules`: modules目录路径（可选，默认为`../../modules`）

### 示例

```bash
# 基本用法
go run . -proto ../../common/msg/pb -modules ../../modules

# 指定所有参数
go run . -proto ../../common/msg/pb -output ../../common/msg/message/handlers -modules ../../modules
```

### 使用批处理文件

```bash
# Windows
generate.bat

# 运行集成测试
.\test_integration.bat

# 测试删除功能
.\test_cleanup.bat
```

## 生成规则

1. **模块检测**: 根据proto文件路径自动检测模块
   - 路径包含`login` → 生成到`modules/login/internal/handlers/`
   - 路径包含`game` → 生成到`modules/game/internal/handlers/`
   - 其他情况 → 默认生成到`modules/game/internal/handlers/`

2. **文件命名**: 将C2S消息名转换为小写并添加`_handler.go`后缀
   - `C2S_Login` → `login_handler.go`
   - `C2S_Reconnect` → `reconnect_handler.go`
   - `C2S_Heart` → `heart_handler.go`

3. **生成内容**: 每个handler文件包含标准的处理函数模板

4. **pb.go生成**: 自动执行protoc命令生成Go代码到`common/msg/message/`目录

5. **自动注册**: 生成handler的同时自动更新相关注册文件

6. **自动删除**: 当proto文件被删除时，自动清理相关文件和注册

## 自动注册的文件

### 1. 模块handler文件 (`modules/{module}/internal/handler.go`)
```go
func InitHandler() {
    handleMsg(&message.C2S_Login{}, handlers.C2S_LoginHandler)
    handleMsg(&message.C2S_Reconnect{}, handlers.C2S_ReconnectHandler)
    // ... 自动添加新的消息注册
}
```

### 2. 路由文件 (`gate/router.go`)
```go
func InitRouter() {
    msg.Processor.SetRouter(&message.C2S_Login{}, login.External.ChanRPC)
    msg.Processor.SetRouter(&message.C2S_Haha{}, game.External.ChanRPC)
    // ... 自动添加新的路由
}
```

### 3. 消息处理器文件 (`common/msg/msg.go`)
```go
func init() {
    Processor.Register(&message.C2S_Login{})
    Processor.Register(&message.C2S_Haha{})
    // ... 自动添加新的消息注册
}
```

## 删除功能

当proto文件被删除时，生成器会自动：

1. **删除handler文件**: 删除对应的`*_handler.go`文件
2. **清理模块注册**: 从`modules/{module}/internal/handler.go`中移除注册
3. **清理路由注册**: 从`gate/router.go`中移除路由
4. **清理消息注册**: 从`common/msg/msg.go`中移除消息注册

### 删除示例

```bash
# 删除proto文件
rm common/msg/pb/game/game.proto

# 运行生成器，会自动清理相关文件
go run . -proto ../../common/msg/pb -modules ../../modules
```

## 注意事项

- 生成器会自动跳过已存在的文件
- 确保proto文件中的C2S消息有正确的message_id选项
- 生成的handler文件需要手动添加具体的业务逻辑
- 确保proto文件语法正确（如分号等）
- 生成的pb.go文件会统一输出到`common/msg/message/`目录
- 自动注册功能会检查文件是否存在，如果不存在会跳过更新
- 自动注册功能会检查是否已经注册过，避免重复注册
- 删除功能会智能检测已删除的proto文件，并清理所有相关文件
- 删除功能会保持其他未删除消息的注册信息不变