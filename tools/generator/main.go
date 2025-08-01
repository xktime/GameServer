package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var sourceDir string

	flag.StringVar(&sourceDir, "source", "", "源目录路径 (包含结构体的目录)")
	flag.Parse()

	if sourceDir == "" {
		fmt.Println("用法: go run main.go -source <源目录>")
		fmt.Println("示例: go run main.go -source ../../modules/game/internal/managers/player")
		fmt.Println("示例: go run main.go -source ../../modules/game/internal/managers/role")
		os.Exit(1)
	}

	// 检查源目录是否存在
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		log.Fatalf("源目录不存在: %s", sourceDir)
	}

	// 自动检测结构体和包名
	structs, packageName, err := AutoDetectStructs(sourceDir)
	if err != nil {
		log.Fatalf("检测结构体失败: %v", err)
	}

	if len(structs) == 0 {
		log.Fatalf("在目录 %s 中未找到包含 ActorMessageHandler 的结构体", sourceDir)
	}

	fmt.Printf("检测到 %d 个结构体: %v\n", len(structs), structs)
	fmt.Printf("包名: %s\n", packageName)

	// 为每个结构体生成代码
	for _, structName := range structs {
		// 自动生成输出文件路径
		outputFile := filepath.Join(sourceDir, fmt.Sprintf("%s_actor.go", strings.ToLower(structName)))

		// 确保输出目录存在
		outputDir := filepath.Dir(outputFile)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("创建输出目录失败: %v", err)
		}

		// 生成代码
		if err := GenerateFromFile(sourceDir, outputFile, structName, packageName); err != nil {
			log.Fatalf("生成代码失败: %v", err)
		}

		fmt.Printf("成功生成代码: %s\n", outputFile)
	}
}
