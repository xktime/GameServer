package handlers

import (
	"gameserver/core/log"
)

// 这是一个测试文件，用于验证生成器是否会跳过已存在的文件
func TestHandler(args []interface{}) {
	log.Debug("这是一个测试handler")
}
