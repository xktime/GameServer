package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var protoDir string
	var outputDir string

	flag.StringVar(&protoDir, "proto", "", "proto文件目录")
	flag.StringVar(&outputDir, "output", "", "输出目录")
	flag.Parse()

	if protoDir == "" {
		fmt.Println("请指定proto文件目录: -proto <目录>")
		os.Exit(1)
	}

	if outputDir == "" {
		outputDir = "../../common/msg/message/handlers"
	}

	fmt.Printf("扫描proto目录: %s\n", protoDir)
	fmt.Printf("输出目录: %s\n", outputDir)

	generator := NewHandlerGenerator(protoDir, outputDir)
	if err := generator.Generate(); err != nil {
		fmt.Printf("生成失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("生成完成!")
}
