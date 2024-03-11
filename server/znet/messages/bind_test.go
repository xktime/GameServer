package messages

import (
	"GameServer/server/common/Tools"
	"fmt"
	"os"
	"testing"
	_ "testing"
)

// 创建连接的时候执行
// todo: 用反射方式加载或自注册解决双向绑定
func TestBind(t *testing.T) {
	pwd, _ := os.Getwd()
	// todo: 根据返回结构体初始化对象
	// todo: 反射messageId类型，缓存进map
	structList := Tools.GetStructListByDir(pwd)
	fmt.Print(structList)
}
