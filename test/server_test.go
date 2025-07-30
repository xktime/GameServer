package test

import (
	"encoding/binary"
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestServer_TcpServer(t *testing.T) {
	conn, err := net.Dial("tcp", "localhost:3563")
	if err != nil {
		panic(err)
	}
	for k := 0; k < 1000000; k++ {
		pbData := &message.C2S_Login{
			LoginType: message.LoginType_DouYin,
			Code:      "123456",
		}
		data, err := proto.Marshal(pbData)
		if err != nil {
			panic(err)
		}
		// len + id+ data
		m := make([]byte, 4+4+len(data))

		// 大端序
		binary.BigEndian.PutUint32(m[0:], uint32(4+len(data)))
		binary.BigEndian.PutUint32(m[4:], getId(pbData)) // id
		copy(m[8:], data)

		// 发送消息
		conn.Write(m)

		// 接收服务器返回的数据
		respBuf := make([]byte, 1024)
		n, err := conn.Read(respBuf)
		if err != nil {
			t.Fatalf("读取服务器返回数据失败: %v", err)
		}
		if n < 8 {
			t.Fatalf("返回数据长度不足: %d", n)
		}
		// 跳过前8字节（长度+消息ID），取后面为protobuf数据
		respData := respBuf[8:n]
		s2cLogin := &message.S2C_Login{}
		if err := proto.Unmarshal(respData, s2cLogin); err != nil {
			t.Fatalf("解析S2C_Login失败: %v", err)
		}
		fmt.Printf("收到S2C_Login: %+v\n", s2cLogin)

		time.Sleep(1 * time.Second)
	}
}

func TestServer_WebSocket(t *testing.T) {
	// 如果服务器在Docker中运行，使用localhost连接
	fmt.Println("正在连接到 WebSocket 服务器: ws://localhost:3653")
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:3653", nil)
	if err != nil {
		t.Fatalf("连接WebSocket服务器失败: %v", err)
	}
	fmt.Println("WebSocket连接成功")
	for k := 0; k < 1000; k++ {
		pbData := &message.C2S_Login{
			LoginType: message.LoginType_WeChat,
			Code:      "123456",
		}

		data, err := proto.Marshal(pbData)
		if err != nil {
			panic(err)
		}
		// len + data
		m := make([]byte, 4+len(data))

		// 大端序
		binary.BigEndian.PutUint32(m[0:], getId(pbData)) // id
		copy(m[4:], data)

		// 发送消息
		conn.WriteMessage(websocket.TextMessage, m)

		// 接收服务器返回的信息
		_, resp, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("读取服务器返回数据失败: %v", err)
		}
		if len(resp) < 4 {
			t.Fatalf("返回数据长度不足: %d", len(resp))
		}
		// 跳过前4字节（消息ID），取后面为protobuf数据
		respData := resp[4:]
		s2cLogin := &message.S2C_Login{}
		if err := proto.Unmarshal(respData, s2cLogin); err != nil {
			t.Fatalf("解析S2C_Login失败: %v", err)
		}
		fmt.Printf("收到S2C_Login: %+v\n", s2cLogin)

		time.Sleep(100 * time.Millisecond)
	}
}

func TestServer_GetMessageId(t *testing.T) {
	fmt.Println("S2C_Login", getId(&message.S2C_Login{}))
	fmt.Println("C2S_Login", getId(&message.C2S_Login{}))

}

func TestServer_SnowFlake(t *testing.T) {
	machineID := 1 // 根据实际情况设置机器ID
	sf := utils.NewSnowflake(int64(machineID))

	// 生成10个唯一ID并输出
	for i := 0; i < 1000; i++ {
		id := sf.GenerateID()
		fmt.Println(id)
	}
}

func getId(m proto.Message) uint32 {
	msgDesc := m.ProtoReflect().Descriptor()
	opts := msgDesc.Options()
	ext := proto.GetExtension(opts, message.E_MessageId)
	return ext.(uint32)
}
