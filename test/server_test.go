package test

import (
	"encoding/binary"
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/modules/game/managers"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestServer_TcpServer(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:3563")
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
	conn, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:3653", nil)
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

		time.Sleep(1 * time.Second)
	}
}

func TestServer_GetMessageId(t *testing.T) {
	fmt.Println("S2C_Login", getId(&message.S2C_Login{}))
	fmt.Println("C2S_Login", getId(&message.C2S_Login{}))

}

func TestServer_GetMessageType(t *testing.T) {
	method := reflect.ValueOf(&managers.UserManager{}).MethodByName("DoLogin")
	if !method.IsValid() {
		t.Fatalf("未找到方法 DoLogin")
	}
	methodType := method.Type()
	fmt.Println(methodType)
}

func TestServer_Func(t *testing.T) {
	actor_manager.Init(2000)
	msg := []interface{}{managers.GetUserManager().DoLogin, "123456", 1}
	_, err := utils.CallMethodWithParams(msg[0], msg[1:]...)
	if err != nil {
		t.Fatalf("调用方法失败: %v", err)
	}
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
