package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

type HandlerGenerator struct {
	ProtoDir   string
	OutputDir  string
	ModulesDir string
}

type MessageInfo struct {
	Name   string
	ID     string
	Module string
}

func NewHandlerGenerator(protoDir, outputDir, modulesDir string) *HandlerGenerator {
	return &HandlerGenerator{
		ProtoDir:   protoDir,
		OutputDir:  outputDir,
		ModulesDir: modulesDir,
	}
}

func (g *HandlerGenerator) Generate() error {
	// 先执行protoc命令生成Go文件
	if err := g.runProtoc(); err != nil {
		return fmt.Errorf("执行protoc失败: %v", err)
	}

	var allMessages []MessageInfo
	var existingHandlers []string

	// 递归扫描所有proto文件
	err := filepath.Walk(g.ProtoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理.proto文件
		if !info.IsDir() && strings.HasSuffix(path, ".proto") {
			messages, err := g.parseProtoFile(path)
			if err != nil {
				return fmt.Errorf("解析proto文件 %s 失败: %v", path, err)
			}
			allMessages = append(allMessages, messages...)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("扫描proto文件失败: %v", err)
	}

	// 获取现有的handler文件列表
	existingHandlers = g.getExistingHandlers()

	// 过滤出C2S开头的消息
	var c2sMessages []MessageInfo
	for _, msg := range allMessages {
		if strings.HasPrefix(msg.Name, "C2S_") {
			c2sMessages = append(c2sMessages, msg)
		}
	}

	// 删除不再存在的handler文件和注册
	if err := g.cleanupRemovedHandlers(c2sMessages, existingHandlers); err != nil {
		return fmt.Errorf("清理已删除的handler失败: %v", err)
	}

	if len(c2sMessages) == 0 {
		fmt.Println("未找到C2S开头的消息")
		return nil
	}

	// 为每个C2S消息生成单独的handler文件
	for _, msg := range c2sMessages {
		if err := g.generateHandlerFile(msg); err != nil {
			return fmt.Errorf("生成handler文件失败: %v", err)
		}
	}

	return nil
}

func (g *HandlerGenerator) parseProtoFile(protoFile string) ([]MessageInfo, error) {
	file, err := os.Open(protoFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []MessageInfo
	scanner := bufio.NewScanner(file)

	// 匹配message定义的正则表达式
	messageRegex := regexp.MustCompile(`^message\s+(\w+)\s*\{`)
	// 匹配message_id的正则表达式
	idRegex := regexp.MustCompile(`option\s*\(message_id\)\s*=\s*(\d+)`)

	var currentMessage string
	var currentID string

	// 根据proto文件路径判断模块
	module := g.detectModuleFromPath(protoFile)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 检查是否是message定义
		if matches := messageRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentMessage = matches[1]
			currentID = ""
		}

		// 检查是否是message_id
		if matches := idRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentID = matches[1]
		}

		// 如果遇到结束大括号，说明message定义结束
		if line == "}" && currentMessage != "" {
			messages = append(messages, MessageInfo{
				Name:   currentMessage,
				ID:     currentID,
				Module: module,
			})
			currentMessage = ""
			currentID = ""
		}
	}

	return messages, scanner.Err()
}

// 新增：根据proto文件路径检测模块
func (g *HandlerGenerator) detectModuleFromPath(protoFile string) string {
	// 获取相对路径
	relPath, err := filepath.Rel(g.ProtoDir, protoFile)
	if err != nil {
		return "game" // 默认返回game
	}

	// 检查路径中是否包含login
	if strings.Contains(relPath, "login") {
		return "login"
	}

	// 检查路径中是否包含game
	if strings.Contains(relPath, "game") {
		return "game"
	}

	// 检查路径中是否包含match
	if strings.Contains(relPath, "match") {
		return "match"
	}

	// 检查路径中是否包含rank
	if strings.Contains(relPath, "rank") {
		return "rank"
	}

	// 默认返回game
	return "game"
}

func (g *HandlerGenerator) generateHandlerFile(msg MessageInfo) error {
	// 根据模块确定输出目录
	moduleOutputDir := filepath.Join(g.ModulesDir, msg.Module, "internal", "handlers")

	// 创建模块输出目录
	if err := os.MkdirAll(moduleOutputDir, 0755); err != nil {
		return fmt.Errorf("创建模块输出目录失败: %v", err)
	}

	// 生成文件名：将C2S_Login转换为login_handler.go
	fileName := strings.ToLower(strings.TrimPrefix(msg.Name, "C2S_")) + "_handler.go"
	outputFile := filepath.Join(moduleOutputDir, fileName)

	// 检查文件是否已存在
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Printf("文件已存在，跳过生成: %s (模块: %s)\n", fileName, msg.Module)
		return nil
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	tmpl := template.Must(template.New("handler").Parse(`
package handlers

import (
	"gameserver/common/msg/message"
	"gameserver/core/gate"
	"gameserver/core/log"
)

// {{.Name}}Handler 处理{{.Name}}消息
func {{.Name}}Handler(args []interface{}) {
	if len(args) < 2 {
		log.Error("{{.Name}}Handler: 参数不足")
		return
	}

	msg, ok := args[0].(*message.{{.Name}})
	if !ok {
		log.Error("{{.Name}}Handler: 消息类型错误")
		return
	}

	agent, ok := args[1].(gate.Agent)
	if !ok {
		log.Error("{{.Name}}Handler: Agent类型错误")
		return
	}

	log.Debug("收到{{.Name}}消息: %v, agent: %v", msg, agent)
	// TODO: 实现具体的业务逻辑
}
`))

	fmt.Printf("生成handler文件: %s (模块: %s)\n", fileName, msg.Module)
	if err := tmpl.Execute(file, msg); err != nil {
		return err
	}

	// 更新相关的注册文件
	if err := g.updateRegistrationFiles(msg); err != nil {
		return fmt.Errorf("更新注册文件失败: %v", err)
	}

	return nil
}

func (g *HandlerGenerator) runProtoc() error {
	// 切换到proto目录
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取当前目录失败: %v", err)
	}

	// 切换到proto目录
	if err := os.Chdir(g.ProtoDir); err != nil {
		return fmt.Errorf("切换到proto目录失败: %v", err)
	}
	defer os.Chdir(originalDir) // 恢复原目录

	// 递归查找所有proto文件
	var protoFiles []string
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理.proto文件
		if !info.IsDir() && strings.HasSuffix(path, ".proto") {
			protoFiles = append(protoFiles, path)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("扫描proto文件失败: %v", err)
	}

	if len(protoFiles) == 0 {
		fmt.Println("未找到proto文件")
		return nil
	}

	fmt.Printf("找到 %d 个proto文件\n", len(protoFiles))

	// 确保输出目录存在
	outputDir := "../message"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 为每个proto文件执行protoc命令
	for _, protoFile := range protoFiles {
		fmt.Printf("处理proto文件: %s\n", protoFile)

		// 获取proto文件所在目录
		protoDir := filepath.Dir(protoFile)
		protoName := filepath.Base(protoFile)

		// 切换到proto文件所在目录
		if protoDir != "." {
			if err := os.Chdir(protoDir); err != nil {
				return fmt.Errorf("切换到目录 %s 失败: %v", protoDir, err)
			}
		}

		// 计算相对于当前目录的输出路径
		relativeOutputDir := "."
		if protoDir != "." {
			// 如果不在根目录，需要调整输出路径
			relativeOutputDir = ".."
		}

		// 执行protoc命令
		cmd := exec.Command("protoc",
			"--proto_path=.",
			"--proto_path=..",
			fmt.Sprintf("--go_out=%s", relativeOutputDir),
			protoName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Printf("执行命令: %s\n", strings.Join(cmd.Args, " "))

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("protoc命令执行失败: %v", err)
		}

		// 恢复原目录
		if protoDir != "." {
			if err := os.Chdir(".."); err != nil {
				return fmt.Errorf("恢复目录失败: %v", err)
			}
		}
	}

	fmt.Println("所有proto文件处理完成")
	return nil
}

// 更新相关的注册文件
func (g *HandlerGenerator) updateRegistrationFiles(msg MessageInfo) error {
	// 1. 更新模块的handler.go文件
	if err := g.updateModuleHandler(msg); err != nil {
		return fmt.Errorf("更新模块handler失败: %v", err)
	}

	// 2. 更新gate/router.go文件
	if err := g.updateRouter(msg); err != nil {
		return fmt.Errorf("更新路由失败: %v", err)
	}

	// 3. 更新common/msg/msg.go文件
	if err := g.updateMsgProcessor(msg); err != nil {
		return fmt.Errorf("更新消息处理器失败: %v", err)
	}

	return nil
}

// 更新模块的handler.go文件
func (g *HandlerGenerator) updateModuleHandler(msg MessageInfo) error {
	handlerFile := filepath.Join(g.ModulesDir, msg.Module, "internal", "handler.go")

	// 检查文件是否存在
	if _, err := os.Stat(handlerFile); err != nil {
		fmt.Printf("模块handler文件不存在，跳过更新: %s\n", handlerFile)
		return nil
	}

	// 读取文件内容
	content, err := os.ReadFile(handlerFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 检查是否已经注册了该消息
	handlerLine := fmt.Sprintf("	handleMsg(&message.%s{}, handlers.%sHandler)", msg.Name, msg.Name)
	if strings.Contains(string(content), handlerLine) {
		fmt.Printf("消息 %s 已在模块handler中注册，跳过\n", msg.Name)
		return nil
	}

	// 在InitHandler函数中添加注册
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inInitHandler := false
	added := false

	for _, line := range lines {
		newLines = append(newLines, line)

		// 检查是否在InitHandler函数中
		if strings.Contains(line, "func InitHandler() {") {
			inInitHandler = true
		}

		// 在InitHandler函数的结束大括号前添加注册
		if inInitHandler && strings.TrimSpace(line) == "}" && !added {
			// 在结束大括号前插入注册语句
			newLines = append(newLines[:len(newLines)-1], fmt.Sprintf("	handleMsg(&message.%s{}, handlers.%sHandler)", msg.Name, msg.Name))
			newLines = append(newLines, "}")
			added = true
		}
	}

	// 写回文件
	if err := os.WriteFile(handlerFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已更新模块handler文件: %s\n", handlerFile)
	return nil
}

// 更新gate/router.go文件
func (g *HandlerGenerator) updateRouter(msg MessageInfo) error {
	routerFile := filepath.Join(g.ModulesDir, "..", "gate", "router.go")

	// 检查文件是否存在
	if _, err := os.Stat(routerFile); err != nil {
		fmt.Printf("路由文件不存在，跳过更新: %s\n", routerFile)
		return nil
	}

	// 读取文件内容
	content, err := os.ReadFile(routerFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 检查是否已经注册了该消息
	routerLine := fmt.Sprintf(`	msg.Processor.SetRouter(&message.%s{}, %s.External.ChanRPC)`, msg.Name, msg.Module)
	if strings.Contains(string(content), routerLine) {
		fmt.Printf("消息 %s 已在路由中注册，跳过\n", msg.Name)
		return nil
	}

	// 在InitRouter函数中添加路由
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inInitRouter := false
	added := false

	for _, line := range lines {
		newLines = append(newLines, line)

		// 检查是否在InitRouter函数中
		if strings.Contains(line, "func InitRouter() {") {
			inInitRouter = true
		}

		// 在InitRouter函数的最后添加路由
		if inInitRouter && strings.Contains(line, "}") && !added {
			newLines = append(newLines, fmt.Sprintf("	msg.Processor.SetRouter(&message.%s{}, %s.External.ChanRPC)", msg.Name, msg.Module))
			added = true
		}
	}

	// 写回文件
	if err := os.WriteFile(routerFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已更新路由文件: %s\n", routerFile)
	return nil
}

// 更新common/msg/msg.go文件
func (g *HandlerGenerator) updateMsgProcessor(msg MessageInfo) error {
	msgFile := filepath.Join(g.ModulesDir, "..", "common", "msg", "msg.go")

	// 检查文件是否存在
	if _, err := os.Stat(msgFile); err != nil {
		fmt.Printf("消息处理器文件不存在，跳过更新: %s\n", msgFile)
		return nil
	}

	// 读取文件内容
	content, err := os.ReadFile(msgFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 检查是否已经注册了该消息
	processorLine := fmt.Sprintf(`	Processor.Register(&message.%s{})`, msg.Name)
	if strings.Contains(string(content), processorLine) {
		fmt.Printf("消息 %s 已在消息处理器中注册，跳过\n", msg.Name)
		return nil
	}

	// 在init函数中添加注册
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inInit := false
	added := false

	for _, line := range lines {
		newLines = append(newLines, line)

		// 检查是否在init函数中
		if strings.Contains(line, "func init() {") {
			inInit = true
		}

		// 在init函数的最后添加注册
		if inInit && strings.Contains(line, "}") && !added {
			newLines = append(newLines, fmt.Sprintf("	Processor.Register(&message.%s{})", msg.Name))
			added = true
		}
	}

	// 写回文件
	if err := os.WriteFile(msgFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已更新消息处理器文件: %s\n", msgFile)
	return nil
}

// 获取现有的handler文件列表
func (g *HandlerGenerator) getExistingHandlers() []string {
	var handlers []string

	// 扫描modules目录下的所有handler文件
	err := filepath.Walk(g.ModulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 只处理handler文件
		if !info.IsDir() && strings.HasSuffix(path, "_handler.go") {
			handlers = append(handlers, path)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("扫描现有handler文件失败: %v\n", err)
	}

	return handlers
}

// 清理已删除的handler文件和注册
func (g *HandlerGenerator) cleanupRemovedHandlers(currentMessages []MessageInfo, existingHandlers []string) error {
	// 创建当前消息的handler文件名映射
	currentHandlerFiles := make(map[string]bool)
	for _, msg := range currentMessages {
		fileName := strings.ToLower(strings.TrimPrefix(msg.Name, "C2S_")) + "_handler.go"
		handlerPath := filepath.Join(g.ModulesDir, msg.Module, "internal", "handlers", fileName)
		currentHandlerFiles[handlerPath] = true
	}

	// 检查每个现有的handler文件
	for _, handlerPath := range existingHandlers {
		// 如果handler文件不在当前消息列表中，则删除它
		if !currentHandlerFiles[handlerPath] {
			if err := g.removeHandlerFile(handlerPath); err != nil {
				return fmt.Errorf("删除handler文件失败: %v", err)
			}
		}
	}

	return nil
}

// 删除handler文件并清理相关注册
func (g *HandlerGenerator) removeHandlerFile(handlerPath string) error {
	// 从handler路径中提取消息名称
	fileName := filepath.Base(handlerPath)
	msgName := "C2S_" + strings.Title(strings.TrimSuffix(strings.TrimSuffix(fileName, "_handler.go"), "_handler"))

	// 从路径中提取模块名称
	pathParts := strings.Split(handlerPath, string(filepath.Separator))
	var moduleName string
	for i, part := range pathParts {
		if part == "modules" && i+1 < len(pathParts) {
			moduleName = pathParts[i+1]
			break
		}
	}

	if moduleName == "" {
		return fmt.Errorf("无法从路径中提取模块名称: %s", handlerPath)
	}

	fmt.Printf("删除handler文件: %s (消息: %s, 模块: %s)\n", fileName, msgName, moduleName)

	// 删除handler文件
	if err := os.Remove(handlerPath); err != nil {
		return fmt.Errorf("删除文件失败: %v", err)
	}

	// 清理相关注册
	msgInfo := MessageInfo{
		Name:   msgName,
		Module: moduleName,
	}

	// 从注册文件中移除该消息
	if err := g.removeFromRegistrationFiles(msgInfo); err != nil {
		return fmt.Errorf("从注册文件中移除失败: %v", err)
	}

	return nil
}

// 从注册文件中移除消息
func (g *HandlerGenerator) removeFromRegistrationFiles(msg MessageInfo) error {
	// 1. 从模块handler文件中移除
	if err := g.removeFromModuleHandler(msg); err != nil {
		return fmt.Errorf("从模块handler中移除失败: %v", err)
	}

	// 2. 从路由文件中移除
	if err := g.removeFromRouter(msg); err != nil {
		return fmt.Errorf("从路由中移除失败: %v", err)
	}

	// 3. 从消息处理器文件中移除
	if err := g.removeFromMsgProcessor(msg); err != nil {
		return fmt.Errorf("从消息处理器中移除失败: %v", err)
	}

	return nil
}

// 从模块handler文件中移除消息
func (g *HandlerGenerator) removeFromModuleHandler(msg MessageInfo) error {
	handlerFile := filepath.Join(g.ModulesDir, msg.Module, "internal", "handler.go")

	// 检查文件是否存在
	if _, err := os.Stat(handlerFile); err != nil {
		return nil // 文件不存在，无需处理
	}

	// 读取文件内容
	content, err := os.ReadFile(handlerFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 要移除的行
	lineToRemove := fmt.Sprintf("	handleMsg(&message.%s{}, handlers.%sHandler)", msg.Name, msg.Name)

	// 移除包含该消息的行
	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		if !strings.Contains(line, lineToRemove) {
			newLines = append(newLines, line)
		}
	}

	// 写回文件
	if err := os.WriteFile(handlerFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已从模块handler文件中移除: %s\n", msg.Name)
	return nil
}

// 从路由文件中移除消息
func (g *HandlerGenerator) removeFromRouter(msg MessageInfo) error {
	routerFile := filepath.Join(g.ModulesDir, "..", "gate", "router.go")

	// 检查文件是否存在
	if _, err := os.Stat(routerFile); err != nil {
		return nil // 文件不存在，无需处理
	}

	// 读取文件内容
	content, err := os.ReadFile(routerFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 要移除的行
	lineToRemove := fmt.Sprintf("	msg.Processor.SetRouter(&message.%s{}, %s.External.ChanRPC)", msg.Name, msg.Module)

	// 移除包含该消息的行
	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		if !strings.Contains(line, lineToRemove) {
			newLines = append(newLines, line)
		}
	}

	// 写回文件
	if err := os.WriteFile(routerFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已从路由文件中移除: %s\n", msg.Name)
	return nil
}

// 从消息处理器文件中移除消息
func (g *HandlerGenerator) removeFromMsgProcessor(msg MessageInfo) error {
	msgFile := filepath.Join(g.ModulesDir, "..", "common", "msg", "msg.go")

	// 检查文件是否存在
	if _, err := os.Stat(msgFile); err != nil {
		return nil // 文件不存在，无需处理
	}

	// 读取文件内容
	content, err := os.ReadFile(msgFile)
	if err != nil {
		return fmt.Errorf("读取文件失败: %v", err)
	}

	// 要移除的行
	lineToRemove := fmt.Sprintf("	Processor.Register(&message.%s{})", msg.Name)

	// 移除包含该消息的行
	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		if !strings.Contains(line, lineToRemove) {
			newLines = append(newLines, line)
		}
	}

	// 写回文件
	if err := os.WriteFile(msgFile, []byte(strings.Join(newLines, "\n")), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}

	fmt.Printf("已从消息处理器文件中移除: %s\n", msg.Name)
	return nil
}
