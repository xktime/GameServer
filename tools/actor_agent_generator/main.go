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
		fmt.Println("请使用 -source 参数指定源目录路径")
		os.Exit(1)
	}

	// 检查源目录是否存在
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		log.Fatalf("源目录不存在: %s", sourceDir)
	}

	// 自动检测结构体和包名
	fmt.Printf("正在扫描目录: %s\n", sourceDir)
	structs, err := AutoDetectStructs(sourceDir)
	if err != nil {
		log.Fatalf("检测结构体失败: %v", err)
	}

	if len(structs) == 0 {
		log.Fatalf("在目录 %s 中未找到包含 ActorMessageHandler 的结构体", sourceDir)
	}

	fmt.Printf("检测到 %d 个结构体\n", len(structs))
	for _, s := range structs {
		fmt.Printf("  - %s (包: %s, 文件: %s)\n", s.Name, s.Package, s.FilePath)
	}

	// 为每个结构体生成代码
	for _, structInfo := range structs {
		structName := structInfo.Name
		packageName := structInfo.Package
		filePath := structInfo.FilePath

		// 检查结构体是否有方法
		hasMethods, err := CheckStructHasMethods(sourceDir, structName)
		if err != nil {
			log.Printf("检查结构体 %s 方法失败: %v", structName, err)
			continue
		}

		if !hasMethods {
			fmt.Printf("跳过结构体 %s: 没有找到方法\n", structName)
			continue
		}

		// 根据结构体所在文件路径确定输出目录
		outputDir := filepath.Dir(filePath)
		outputFile := filepath.Join(outputDir, fmt.Sprintf("%s_actor.go", strings.ToLower(structName)))

		// 确保输出目录存在
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
