package test

import (
	"encoding/binary"
	"fmt"
	"gameserver/common/config"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	actor_manager "gameserver/core/actor"
	"gameserver/modules/login"
	"net"
	"sync"
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
	const total = 1000
	const batchSize = 10

	for batchStart := 0; batchStart < total; batchStart += batchSize {
		var wg sync.WaitGroup
		for i := 0; i < batchSize && batchStart+i < total; i++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				fmt.Printf("=== 第 %d 次测试 ===\n", k+1)

				// 连接WebSocket服务器
				fmt.Println("正在连接到 WebSocket 服务器: ws://localhost:3653")
				conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:3653", nil)
				if err != nil {
					t.Logf("连接WebSocket服务器失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}
				fmt.Println("WebSocket连接成功")

				// 使用defer确保连接被正确关闭
				defer func() {
					if conn != nil {
						// 发送关闭消息
						conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
						conn.Close()
					}
				}()

				// 1. 发送登录请求
				fmt.Println("发送登录请求...")
				loginMsg := &message.C2S_Login{
					LoginType: message.LoginType_WeChat,
					Code:      "123456",
					ServerId:  1,
				}

				if err := sendMessage(conn, loginMsg); err != nil {
					t.Logf("发送登录消息失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}

				// 接收登录响应
				resp, err := receiveMessage(conn)
				if err != nil {
					t.Logf("接收登录响应失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}

				s2cLogin := &message.S2C_Login{}
				if err := parseMessage(resp, s2cLogin); err != nil {
					t.Logf("解析登录响应失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}
				fmt.Printf("登录成功: %+v\n", s2cLogin)

				// 2. 登录成功后，自动请求匹配
				fmt.Println("登录成功，开始请求匹配...")
				startMatchMsg := &message.C2S_StartMatch{
					Type: 1, // 匹配类型1
				}

				if err := sendMessage(conn, startMatchMsg); err != nil {
					t.Logf("发送匹配请求失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}

				// 接收匹配开始响应
				resp, err = receiveMessage(conn)
				if err != nil {
					t.Logf("接收匹配开始响应失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}

				s2cStartMatch := &message.S2C_StartMatch{}
				if err := parseMessage(resp, s2cStartMatch); err != nil {
					t.Logf("解析匹配开始响应失败: %v", err)
					fmt.Printf("=== 第 %d 次测试跳过 ===\n\n", k+1)
					return
				}
				fmt.Printf("匹配请求响应: %+v\n", s2cStartMatch)

				// 3. 等待匹配结果（最多等待20秒）
				fmt.Println("等待匹配结果...")
				timeout := time.After(20 * time.Second)
				matchResultReceived := false
				consecutiveErrors := 0
				maxConsecutiveErrors := 3

				for !matchResultReceived {
					select {
					case <-timeout:
						fmt.Println("等待匹配结果超时")
						matchResultReceived = true
					default:
						// 检查连接健康状态
						if !isConnectionHealthy(conn) {
							t.Logf("连接不健康，停止等待")
							matchResultReceived = true
							break
						}

						// 尝试接收匹配结果消息
						if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
							t.Logf("设置读取超时失败: %v", err)
						}

						resp, err := receiveMessage(conn)
						if err != nil {
							consecutiveErrors++

							// 检查是否是连接关闭错误
							if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
								t.Logf("连接已关闭: %v", err)
								matchResultReceived = true
								break
							}

							if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
								// 超时，继续等待
								consecutiveErrors = 0 // 重置错误计数
								continue
							}

							// 其他错误，记录日志并继续
							t.Logf("接收消息时出错: %v (连续错误次数: %d)", err, consecutiveErrors)

							// 如果连续错误次数过多，停止尝试
							if consecutiveErrors >= maxConsecutiveErrors {
								t.Logf("连续错误次数过多，停止等待")
								matchResultReceived = true
								break
							}

							// 短暂等待后重试
							time.Sleep(100 * time.Millisecond)
							continue
						}

						// 成功接收消息，重置错误计数
						consecutiveErrors = 0

						// 尝试解析为匹配结果消息
						matchResult := &message.S2C_MatchResult{}
						if err := parseMessage(resp, matchResult); err == nil {
							fmt.Printf("收到匹配结果: %+v\n", matchResult)
							fmt.Printf("房间ID: %d, 玩家数量: %d\n", matchResult.RoomId, len(matchResult.PlayerInfos))

							// 打印玩家信息
							for i, player := range matchResult.PlayerInfos {
								fmt.Printf("  玩家%d: ID=%d, 是否机器人=%v\n", i+1, player.PlayerId, player.IsRobot)
							}

							matchResultReceived = true
							break
						}

						// 如果不是匹配结果消息，尝试解析为其他消息
						fmt.Printf("收到其他消息，长度: %d\n", len(resp))
					}
				}

				// 等待一段时间后关闭连接
				time.Sleep(500 * time.Millisecond)
				fmt.Printf("=== 第 %d 次测试完成 ===\n\n", k+1)
			}(batchStart + i)
		}
		wg.Wait()
	}
}

func TestServer_GetMessageId(t *testing.T) {
	fmt.Println("S2C_Login", getId(&message.S2C_Login{}))
	fmt.Println("C2S_Login", getId(&message.C2S_Login{}))
	fmt.Println("C2S_StartMatch", getId(&message.C2S_StartMatch{}))
	fmt.Println("S2C_StartMatch", getId(&message.S2C_StartMatch{}))
	fmt.Println("S2C_MatchResult", getId(&message.S2C_MatchResult{}))
}

// TestServer_MatchFlow 专门测试匹配流程
func TestServer_MatchFlow(t *testing.T) {
	fmt.Println("=== 开始测试匹配流程 ===")

	// 连接WebSocket服务器
	fmt.Println("正在连接到 WebSocket 服务器: ws://localhost:3653")
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:3653", nil)
	if err != nil {
		t.Fatalf("连接WebSocket服务器失败: %v", err)
	}

	// 使用defer确保连接被正确关闭
	defer func() {
		if conn != nil {
			// 发送关闭消息
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			conn.Close()
		}
	}()

	fmt.Println("WebSocket连接成功")

	// 1. 发送登录请求
	fmt.Println("发送登录请求...")
	loginMsg := &message.C2S_Login{
		LoginType: message.LoginType_WeChat,
		Code:      "123456",
		ServerId:  1,
	}

	if err := sendMessage(conn, loginMsg); err != nil {
		t.Fatalf("发送登录消息失败: %v", err)
	}

	// 接收登录响应
	resp, err := receiveMessage(conn)
	if err != nil {
		t.Fatalf("接收登录响应失败: %v", err)
	}

	s2cLogin := &message.S2C_Login{}
	if err := parseMessage(resp, s2cLogin); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	fmt.Printf("登录成功: %+v\n", s2cLogin)

	// 2. 登录成功后，自动请求匹配
	fmt.Println("登录成功，开始请求匹配...")
	startMatchMsg := &message.C2S_StartMatch{
		Type: 1, // 匹配类型1
	}

	if err := sendMessage(conn, startMatchMsg); err != nil {
		t.Fatalf("发送匹配请求失败: %v", err)
	}

	// 接收匹配开始响应
	resp, err = receiveMessage(conn)
	if err != nil {
		t.Fatalf("接收匹配开始响应失败: %v", err)
	}

	s2cStartMatch := &message.S2C_StartMatch{}
	if err := parseMessage(resp, s2cStartMatch); err != nil {
		t.Fatalf("解析匹配开始响应失败: %v", err)
	}
	fmt.Printf("匹配请求响应: %+v\n", s2cStartMatch)

	if !s2cStartMatch.Result {
		t.Logf("匹配请求失败，可能已经在匹配队列中")
		return
	}

	// 3. 等待匹配结果（最多等待60秒）
	fmt.Println("等待匹配结果...")
	timeout := time.After(60 * time.Second)
	matchResultReceived := false
	consecutiveErrors := 0
	maxConsecutiveErrors := 5

	for !matchResultReceived {
		select {
		case <-timeout:
			fmt.Println("等待匹配结果超时")
			matchResultReceived = true
		default:
			// 检查连接健康状态
			if !isConnectionHealthy(conn) {
				t.Logf("连接不健康，停止等待")
				matchResultReceived = true
				break
			}

			// 尝试接收匹配结果消息
			if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Logf("设置读取超时失败: %v", err)
			}

			resp, err := receiveMessage(conn)
			if err != nil {
				consecutiveErrors++

				// 检查是否是连接关闭错误
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					t.Logf("连接已关闭: %v", err)
					matchResultReceived = true
					break
				}

				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 超时，继续等待
					consecutiveErrors = 0 // 重置错误计数
					fmt.Print(".")
					continue
				}

				// 其他错误，记录日志并继续
				t.Logf("接收消息时出错: %v (连续错误次数: %d)", err, consecutiveErrors)

				// 如果连续错误次数过多，停止尝试
				if consecutiveErrors >= maxConsecutiveErrors {
					t.Logf("连续错误次数过多，停止等待")
					matchResultReceived = true
					break
				}

				// 短暂等待后重试
				time.Sleep(200 * time.Millisecond)
				continue
			}

			// 成功接收消息，重置错误计数
			consecutiveErrors = 0

			// 尝试解析为匹配结果消息
			matchResult := &message.S2C_MatchResult{}
			if err := parseMessage(resp, matchResult); err == nil {
				fmt.Printf("\n收到匹配结果: %+v\n", matchResult)
				fmt.Printf("房间ID: %d, 玩家数量: %d\n", matchResult.RoomId, len(matchResult.PlayerInfos))

				// 打印玩家信息
				for i, player := range matchResult.PlayerInfos {
					fmt.Printf("  玩家%d: ID=%d, 是否机器人=%v\n", i+1, player.PlayerId, player.IsRobot)
				}

				matchResultReceived = true
				break
			}

			// 如果不是匹配结果消息，尝试解析为其他消息
			fmt.Printf("收到其他消息，长度: %d\n", len(resp))
		}
	}

	fmt.Println("=== 匹配流程测试完成 ===")
}

func TestServer_Func(t *testing.T) {
	actor_manager.Init(2000)
	_, err := utils.CallMethodWithParams(&login.LoginExternal{}, "InitExternal")
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

func TestServer_Config(t *testing.T) {
	// 初始化配置管理器
	config.InitGlobalConfig("../conf/config")

	// 测试加载配置文件
	t.Run("LoadConfig", func(t *testing.T) {
		if err := config.LoadConfig("items.json"); err != nil {
			t.Fatalf("加载物品配置失败: %v", err)
		}

		if err := config.LoadConfig("monsters.json"); err != nil {
			t.Fatalf("加载怪物配置失败: %v", err)
		}
	})

	// 测试根据文件名和ID获取配置
	t.Run("GetConfig", func(t *testing.T) {
		// 测试获取存在的配置
		if item, exists := config.GetConfig("items.json", "1001"); !exists {
			t.Error("未找到物品配置 1001")
		} else {
			itemConfig, ok := item.(map[string]interface{})
			if !ok {
				t.Error("物品配置类型错误")
			}
			if name, ok := itemConfig["name"].(string); !ok || name != "铁剑" {
				t.Errorf("物品名称错误，期望: 铁剑, 实际: %s", name)
			}
		}

		// 测试获取不存在的配置
		if _, exists := config.GetConfig("items.json", "9999"); exists {
			t.Error("找到了不存在的物品配置")
		}

		// 测试获取怪物配置
		if monster, exists := config.GetConfig("monsters.json", "1001"); !exists {
			t.Error("未找到怪物配置 1001")
		} else {
			monsterConfig, ok := monster.(map[string]interface{})
			if !ok {
				t.Error("怪物配置类型错误")
			}
			if name, ok := monsterConfig["name"].(string); !ok || name != "史莱姆" {
				t.Errorf("怪物名称错误，期望: 史莱姆, 实际: %s", name)
			}
		}
	})

	// 测试根据ID自动查找配置
	t.Run("GetConfigByID", func(t *testing.T) {
		if filename, config, exists := config.GetConfigByID("1001"); !exists {
			t.Error("未找到配置 1001")
		} else {
			if filename != "items.json" {
				t.Errorf("文件名错误，期望: items.json, 实际: %s", filename)
			}
			if config == nil {
				t.Error("配置为空")
			}
		}

		// 测试查找不存在的ID
		if _, _, exists := config.GetConfigByID("9999"); exists {
			t.Error("找到了不存在的配置")
		}
	})

	// 测试获取所有配置
	t.Run("GetAllConfigs", func(t *testing.T) {
		if allItems, exists := config.GetAllConfigs("items.json"); !exists {
			t.Error("未找到所有物品配置")
		} else {
			expectedCount := 4 // 根据示例文件中的物品数量
			if len(allItems) != expectedCount {
				t.Errorf("物品配置数量错误，期望: %d, 实际: %d", expectedCount, len(allItems))
			}
		}

		// 测试获取不存在的文件
		if _, exists := config.GetAllConfigs("nonexistent.json"); exists {
			t.Error("找到了不存在的文件配置")
		}
	})

	// 测试便捷方法
	t.Run("ConvenienceMethods", func(t *testing.T) {
		// 测试获取物品名称
		if name, exists := config.GetItemName("1001"); !exists {
			t.Error("未找到物品名称")
		} else if name != "铁剑" {
			t.Errorf("物品名称错误，期望: 铁剑, 实际: %s", name)
		}

		// 测试获取怪物名称
		if name, exists := config.GetMonsterName("1001"); !exists {
			t.Error("未找到怪物名称")
		} else if name != "史莱姆" {
			t.Errorf("怪物名称错误，期望: 史莱姆, 实际: %s", name)
		}

		// 测试获取物品配置
		if itemConfig, exists := config.GetItemConfig("1001"); !exists {
			t.Error("未找到物品配置")
		} else {
			if name, ok := itemConfig["name"].(string); !ok || name != "铁剑" {
				t.Errorf("物品配置名称错误，期望: 铁剑, 实际: %s", name)
			}
		}

		// 测试获取怪物配置
		if monsterConfig, exists := config.GetMonsterConfig("1001"); !exists {
			t.Error("未找到怪物配置")
		} else {
			if name, ok := monsterConfig["name"].(string); !ok || name != "史莱姆" {
				t.Errorf("怪物配置名称错误，期望: 史莱姆, 实际: %s", name)
			}
		}
	})

	// 测试列出已加载文件
	t.Run("ListLoadedFiles", func(t *testing.T) {
		loadedFiles := config.ListLoadedFiles()
		expectedFiles := []string{"items.json", "monsters.json"}

		if len(loadedFiles) != len(expectedFiles) {
			t.Errorf("已加载文件数量错误，期望: %d, 实际: %d", len(expectedFiles), len(loadedFiles))
		}

		for _, expectedFile := range expectedFiles {
			found := false
			for _, loadedFile := range loadedFiles {
				if loadedFile == expectedFile {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("未找到预期的文件: %s", expectedFile)
			}
		}
	})

	// 测试配置重载
	t.Run("ReloadConfig", func(t *testing.T) {
		if err := config.ReloadConfig("items.json"); err != nil {
			t.Errorf("重新加载配置失败: %v", err)
		}

		// 验证重载后配置仍然可用
		if _, exists := config.GetConfig("items.json", "1001"); !exists {
			t.Error("重载后无法获取配置")
		}
	})

	// 测试加载所有配置
	t.Run("LoadAllConfigs", func(t *testing.T) {
		// 先清空当前配置
		config.InitGlobalConfig("../conf/config")

		if err := config.LoadAllConfigs(); err != nil {
			t.Errorf("加载所有配置失败: %v", err)
		}

		// 验证所有JSON文件都被加载
		loadedFiles := config.ListLoadedFiles()
		if len(loadedFiles) == 0 {
			t.Error("未加载任何配置文件")
		}
	})

	// 测试并发访问
	t.Run("ConcurrentAccess", func(t *testing.T) {
		done := make(chan bool, 10)

		// 启动多个goroutine并发访问配置
		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()

				// 并发读取配置
				if _, exists := config.GetConfig("items.json", "1001"); !exists {
					t.Error("并发访问时未找到配置")
				}

				if name, exists := config.GetItemName("1001"); !exists {
					t.Error("并发访问时未找到物品名称")
				} else if name != "铁剑" {
					t.Errorf("并发访问时物品名称错误: %s", name)
				}
			}()
		}

		// 等待所有goroutine完成
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Log("配置系统测试完成")
}

// sendMessage 封装发送消息到WebSocket连接的方法
func sendMessage(conn *websocket.Conn, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	// 构建消息格式: 消息ID + protobuf数据
	m := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(m[0:], getId(msg)) // 消息ID
	copy(m[4:], data)                             // protobuf数据

	// 发送消息
	return conn.WriteMessage(websocket.BinaryMessage, m)
}

// receiveMessage 封装接收消息的方法
func receiveMessage(conn *websocket.Conn) ([]byte, error) {
	// 检查连接状态
	if conn == nil {
		return nil, fmt.Errorf("连接为空")
	}

	// 检查连接是否已经关闭
	if conn.NetConn() == nil {
		return nil, fmt.Errorf("底层连接已关闭")
	}

	_, resp, err := conn.ReadMessage()
	if err != nil {
		// 检查是否是连接关闭错误
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			return nil, fmt.Errorf("连接已关闭: %v", err)
		}
		// 检查是否是网络错误
		if websocket.IsUnexpectedCloseError(err) {
			return nil, fmt.Errorf("连接意外关闭: %v", err)
		}
		return nil, fmt.Errorf("读取消息失败: %v", err)
	}

	if len(resp) < 4 {
		return nil, fmt.Errorf("返回数据长度不足: %d", len(resp))
	}
	return resp, nil
}

// isConnectionHealthy 检查连接是否健康
func isConnectionHealthy(conn *websocket.Conn) bool {
	if conn == nil {
		return false
	}

	// 尝试发送ping消息来检查连接状态
	err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
	return err == nil
}

// parseMessage 解析接收到的消息
func parseMessage(resp []byte, msg proto.Message) error {
	// 跳过前4字节（消息ID），取后面为protobuf数据
	respData := resp[4:]
	return proto.Unmarshal(respData, msg)
}
