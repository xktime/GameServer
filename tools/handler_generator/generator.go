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
	ProtoDir  string
	OutputDir string
}

type MessageInfo struct {
	Name string
	ID   string
}

func NewHandlerGenerator(protoDir, outputDir string) *HandlerGenerator {
	return &HandlerGenerator{
		ProtoDir:  protoDir,
		OutputDir: outputDir,
	}
}

func (g *HandlerGenerator) Generate() error {
	// 先执行protoc命令生成Go文件
	if err := g.runProtoc(); err != nil {
		return fmt.Errorf("执行protoc失败: %v", err)
	}

	// 创建输出目录
	if err := os.MkdirAll(g.OutputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 扫描proto文件
	protoFiles, err := filepath.Glob(filepath.Join(g.ProtoDir, "*.proto"))
	if err != nil {
		return fmt.Errorf("扫描proto文件失败: %v", err)
	}

	var allMessages []MessageInfo

	// 解析所有proto文件
	for _, protoFile := range protoFiles {
		messages, err := g.parseProtoFile(protoFile)
		if err != nil {
			return fmt.Errorf("解析proto文件 %s 失败: %v", protoFile, err)
		}
		allMessages = append(allMessages, messages...)
	}

	// 过滤出C2S开头的消息
	var c2sMessages []MessageInfo
	for _, msg := range allMessages {
		if strings.HasPrefix(msg.Name, "C2S_") {
			c2sMessages = append(c2sMessages, msg)
		}
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
				Name: currentMessage,
				ID:   currentID,
			})
			currentMessage = ""
			currentID = ""
		}
	}

	return messages, scanner.Err()
}

func (g *HandlerGenerator) generateHandlerFile(msg MessageInfo) error {
	// 生成文件名：将C2S_Login转换为login_handler.go
	fileName := strings.ToLower(strings.TrimPrefix(msg.Name, "C2S_")) + "_handler.go"
	outputFile := filepath.Join(g.OutputDir, fileName)

	// 检查文件是否已存在
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Printf("文件已存在，跳过生成: %s\n", fileName)
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

	// TODO: 实现具体的业务逻辑
	log.Debug("收到{{.Name}}消息: %v", msg)
	
	// 打印agent信息以避免not used警告
	log.Debug("Agent信息: %v", agent)
	
	// 示例：发送响应
	// response := &message.S2C_{{.Name}}Response{}
	// agent.WriteMsg(response)
}
`))

	return tmpl.Execute(file, msg)
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

	// 执行protoc命令
	cmd := exec.Command("protoc", "--go_out=./../message", "--go_opt=paths=source_relative", "*.proto")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行命令: %s\n", strings.Join(cmd.Args, " "))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("protoc命令执行失败: %v", err)
	}

	fmt.Println("protoc命令执行成功")
	return nil
}
