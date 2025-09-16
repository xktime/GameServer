package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// stringSlice 实现 flag.Value 接口，用于处理字符串数组参数
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = strings.Split(value, ",")
	return nil
}

func main() {
	var protoDir string
	var outputDir string
	var modulesDir string
	var ingoreFile []string

	flag.StringVar(&protoDir, "proto", "", "proto文件目录")
	flag.StringVar(&outputDir, "output", "", "输出目录")
	flag.StringVar(&modulesDir, "modules", "", "modules目录")
	flag.Var((*stringSlice)(&ingoreFile), "ingoreFile", "忽略文件列表，用逗号分隔")
	flag.Parse()

	if protoDir == "" {
		fmt.Println("请指定proto文件目录: -proto <目录>")
		os.Exit(1)
	}

	if outputDir == "" {
		outputDir = "../../common/msg/message/handlers"
	}

	if modulesDir == "" {
		modulesDir = "../../modules"
	}

	if len(ingoreFile) == 0 {
		ingoreFile = []string{"rpc.proto"}
	}

	fmt.Printf("扫描proto目录: %s\n", protoDir)
	fmt.Printf("输出目录: %s\n", outputDir)
	fmt.Printf("Modules目录: %s\n", modulesDir)

	generator := NewHandlerGenerator(protoDir, outputDir, modulesDir, ingoreFile)
	if err := generator.Generate(); err != nil {
		fmt.Printf("生成失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("生成完成!")
}
