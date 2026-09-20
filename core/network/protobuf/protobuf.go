package protobuf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"gameserver/common/msg/message"
	"gameserver/core/chanrpc"
	"gameserver/core/log"
	"gameserver/core/network/models"
	"math"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// -------------------------
// | 回复标志（1 字节）| 序列号（4 字节）| 消息编号（4 字节）| Protobuf 消息体 |
// -------------------------
type Processor struct {
	littleEndian bool
	msgInfo      map[uint32]*MsgInfo
	msgID        map[reflect.Type]uint32
	routeFilter  RouteFilter
}

type MsgInfo struct {
	msgType    reflect.Type
	msgRouter  *chanrpc.Server
	msgHandler MsgHandler
}

type MsgHandler func([]interface{})

// RouteFilter 在消息交给处理函数或模块路由前执行，返回错误即终止本次分发。
type RouteFilter func(msg interface{}, routeContext interface{}) error

type MsgRaw struct {
	msgID      uint32
	msgRawData []byte
}

func NewProcessor() *Processor {
	p := new(Processor)
	p.littleEndian = false
	p.msgID = make(map[reflect.Type]uint32)
	p.msgInfo = make(map[uint32]*MsgInfo)
	return p
}

// 不能在消息路由、序列化或反序列化期间调用此方法修改配置。
func (p *Processor) SetByteOrder(littleEndian bool) {
	p.littleEndian = littleEndian
}

// 不能在消息路由、序列化或反序列化期间调用此方法修改配置。
func (p *Processor) Register(msg proto.Message) uint32 {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("protobuf message pointer required")
	}
	if _, ok := p.msgID[msgType]; ok {
		log.Fatal("message %s is already registered", msgType)
	}
	if len(p.msgInfo) >= math.MaxUint16 {
		log.Fatal("too many protobuf messages (max = %v)", math.MaxUint16)
	}

	i := new(MsgInfo)
	i.msgType = msgType
	id := getId(msg)
	if p.msgInfo[id] != nil {
		log.Fatal("message id %v is already registered", id)
	}
	p.msgInfo[id] = i
	p.msgID[msgType] = id
	return id
}

// 不能在消息路由、序列化或反序列化期间调用此方法修改配置。
func (p *Processor) SetRouter(msg proto.Message, msgRouter *chanrpc.Server) {
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		log.Fatal("message %s not registered", msgType)
	}
	p.msgInfo[id].msgRouter = msgRouter
}

// 不能在消息路由、序列化或反序列化期间调用此方法修改配置。
func (p *Processor) SetHandler(msg proto.Message, msgHandler MsgHandler) {
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		log.Fatal("message %s not registered", msgType)
	}

	p.msgInfo[id].msgHandler = msgHandler
}

// SetRouteFilter 配置分发前过滤器；必须在 Processor 开始处理连接前调用。
func (p *Processor) SetRouteFilter(filter RouteFilter) {
	p.routeFilter = filter
}

// 支持并发调用。
func (p *Processor) Route(msgSeq *models.MsgWithSeq, routeContext interface{}) error {
	// 处理带序列号的消息
	msg := msgSeq.MsgData
	if msgWithSeq, ok := msg.(*models.MsgWithSeq); ok {
		if msgWithSeq.MsgID >= uint32(len(p.msgInfo)) {
			return fmt.Errorf("message id %v not registered", msgWithSeq.MsgID)
		}
		return nil
	}

	// 处理已携带消息编号的原始消息。
	if msgRaw, ok := msg.(MsgRaw); ok {
		if msgRaw.msgID >= uint32(len(p.msgInfo)) {
			return fmt.Errorf("message id %v not registered", msgRaw.msgID)
		}
		return nil
	}

	// 根据消息类型查找 Protobuf 消息的路由信息。
	msgType := reflect.TypeOf(msg)
	id, ok := p.msgID[msgType]
	if !ok {
		return fmt.Errorf("message %s not registered", msgType)
	}
	i := p.msgInfo[id]
	if p.routeFilter != nil {
		if err := p.routeFilter(msg, routeContext); err != nil {
			return err
		}
	}
	if i.msgHandler != nil {
		i.msgHandler([]interface{}{msg, routeContext, msgSeq.Seq})
	}
	if i.msgRouter != nil {
		i.msgRouter.Go(msgType, msg, routeContext, msgSeq.Seq)
	}
	return nil
}

// 1(是否是回复消息) + 4(序列号) + 4(消息ID) + data
// 支持并发调用。
func (p *Processor) Unmarshal(data []byte) (*models.MsgWithSeq, error) {
	if len(data) < 9 { // 包头至少需要 9 字节：标志位 1 字节、消息编号 4 字节、序列号 4 字节。
		return nil, errors.New("protobuf data too short")
	}

	// 解析消息格式: 1(是否是回复消息) + 4(序列号) + 4(消息ID) + data
	isReply := data[0] != 0

	var seq uint32
	var id uint32

	if p.littleEndian {
		seq = binary.LittleEndian.Uint32(data[1:5])
		id = binary.LittleEndian.Uint32(data[5:9])
	} else {
		seq = binary.BigEndian.Uint32(data[1:5])
		id = binary.BigEndian.Uint32(data[5:9])
	}

	if p.msgInfo[id] == nil {
		return nil, fmt.Errorf("message id %v not registered", id)
	}

	// 创建并反序列化消息体。
	i := p.msgInfo[id]

	msg := reflect.New(i.msgType.Elem()).Interface()
	err := proto.Unmarshal(data[9:], msg.(proto.Message))
	if err != nil {
		return nil, err
	}
	// 返回带序列号信息的消息结构
	return &models.MsgWithSeq{
		IsReply: isReply,
		Seq: func() uint32 {
			if isReply {
				return seq
			} else {
				return 0
			}
		}(),
		MsgID:   id,
		MsgData: msg,
	}, nil
}

// 支持并发调用。
func (p *Processor) Marshal(msg interface{}, seq uint32) ([][]byte, error) {
	pbMsg := msg.(proto.Message)
	_id := getId(pbMsg)

	// 构建新的消息格式: 1(是否是回复消息) + 4(序列号) + 4(消息ID) + data
	header := make([]byte, 9)

	// 1. 是否是回复消息
	isReply := seq != 0
	if isReply {
		header[0] = 1
	} else {
		header[0] = 0
	}

	// 2. 序列号
	if p.littleEndian {
		binary.LittleEndian.PutUint32(header[1:5], seq)
		binary.LittleEndian.PutUint32(header[5:9], _id)
	} else {
		binary.BigEndian.PutUint32(header[1:5], seq)
		binary.BigEndian.PutUint32(header[5:9], _id)
	}

	// 3. protobuf数据
	data, err := proto.Marshal(pbMsg)
	if err != nil {
		return nil, err
	}

	return [][]byte{header, data}, nil
}

// 支持并发调用。
func (p *Processor) Range(f func(id uint16, t reflect.Type)) {
	for id, i := range p.msgInfo {
		f(uint16(id), i.msgType)
	}
}

// getId 从协议描述符的 message_id 扩展中读取消息编号。
func getId(m proto.Message) uint32 {
	msgDesc := m.ProtoReflect().Descriptor()
	opts := msgDesc.Options()
	ext := proto.GetExtension(opts, message.E_MessageId)
	return ext.(uint32)
}
