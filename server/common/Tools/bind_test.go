package Tools

import (
	"fmt"
	"os"
	"testing"
	_ "testing"
)

// 创建连接的时候执行
func TestBind(t *testing.T) {
	pwd, _ := os.Getwd()
	structList := GetStructListByDir(pwd)
	fmt.Print(structList)
}
