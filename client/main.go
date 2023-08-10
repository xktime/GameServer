package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	fmt.Println("Client start")
	time.Sleep(1 * time.Second)
	// 连接远程服务器 得到conn
	conn, err := net.Dial("tcp", "127.0.0.1:8999")
	if err != nil {
		fmt.Println("client start err, exit")
		return
	}

	// 调用Write方法写入数据
	for {
		_, err := conn.Write([]byte("hello"))
		if err != nil {
			fmt.Println("write conn err", err)
			return
		}

		buf := make([]byte, 512)
		cnt, err := conn.Read(buf)
		if err != nil {
			fmt.Println("read buf err", err)
			return
		}
		fmt.Printf("server call back:%s,cnt=%d \n", buf, cnt)
		time.Sleep(1 * time.Second)

	}
}
