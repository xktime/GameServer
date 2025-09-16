package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"gameserver/core/chanrpc"
	"gameserver/core/log"
	"gameserver/core/network/models"
	"reflect"
)

type Processor struct {
	msgInfo map[string]*MsgInfo
}

type MsgInfo struct {
	msgType    reflect.Type
	msgRouter  *chanrpc.Server
	msgHandler MsgHandler
}

type MsgHandler func([]interface{})

type MsgRaw struct {
	msgID      string
	msgRawData json.RawMessage
}

func NewProcessor() *Processor {
	p := new(Processor)
	p.msgInfo = make(map[string]*MsgInfo)
	return p
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) Register(msg interface{}) string {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if msgID == "" {
		log.Fatal("unnamed json message")
	}
	if _, ok := p.msgInfo[msgID]; ok {
		log.Fatal("message %v is already registered", msgID)
	}

	i := new(MsgInfo)
	i.msgType = msgType
	p.msgInfo[msgID] = i
	return msgID
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) SetRouter(msg interface{}, msgRouter *chanrpc.Server) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %v not registered", msgID)
	}

	i.msgRouter = msgRouter
}

// It's dangerous to call the method on routing or marshaling (unmarshaling)
func (p *Processor) SetHandler(msg interface{}, msgHandler MsgHandler) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		log.Fatal("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		log.Fatal("message %v not registered", msgID)
	}

	i.msgHandler = msgHandler
}

// goroutine safe
func (p *Processor) Route(msgSeq *models.MsgWithSeq, userData interface{}) error {
	// 处理带序列号的消息
	msg := msgSeq.MsgData
	if _, ok := msg.(*models.MsgWithSeq); ok {
		// JSON消息的MsgID为0，需要通过MsgData来获取实际的msgID
		// 这里简化处理，直接返回nil表示处理完成
		return nil
	}

	// raw
	if msgRaw, ok := msg.(MsgRaw); ok {
		_, ok := p.msgInfo[msgRaw.msgID]
		if !ok {
			return fmt.Errorf("message %v not registered", msgRaw.msgID)
		}

		return nil
	}

	// json
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	i, ok := p.msgInfo[msgID]
	if !ok {
		return fmt.Errorf("message %v not registered", msgID)
	}
	if i.msgHandler != nil {
		i.msgHandler([]interface{}{msg, userData, msgSeq.Seq})
	}
	if i.msgRouter != nil {
		i.msgRouter.Go(msgType, msg, userData, msgSeq.Seq)
	}
	return nil
}

// goroutine safe
func (p *Processor) Unmarshal(data []byte) (*models.MsgWithSeq, error) {
	// 解析JSON消息格式: {"isReply": bool, "seq": number, "msgID": string, "data": {...}}
	var msgWithSeq struct {
		IsReply bool            `json:"isReply"`
		Seq     uint32          `json:"seq"`
		MsgID   string          `json:"msgID"`
		Data    json.RawMessage `json:"data"`
	}

	err := json.Unmarshal(data, &msgWithSeq)
	if err != nil {
		return nil, err
	}

	// 检查消息是否已注册
	i, ok := p.msgInfo[msgWithSeq.MsgID]
	if !ok {
		return nil, fmt.Errorf("message %v not registered", msgWithSeq.MsgID)
	}

	// 解析消息数据
	msg := reflect.New(i.msgType.Elem()).Interface()
	err = json.Unmarshal(msgWithSeq.Data, msg)
	if err != nil {
		return nil, err
	}

	// 返回带序列号信息的消息结构
	return &models.MsgWithSeq{
		IsReply: msgWithSeq.IsReply,
		Seq:     msgWithSeq.Seq,
		MsgID:   uint32(0), // JSON消息使用字符串ID，这里设为0
		MsgData: msg,
	}, nil
}

// goroutine safe
func (p *Processor) Marshal(msg interface{}, seq uint32) ([][]byte, error) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return nil, errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if _, ok := p.msgInfo[msgID]; !ok {
		return nil, fmt.Errorf("message %v not registered", msgID)
	}

	// 构建新的消息格式: {"isReply": bool, "seq": number, "msgID": string, "data": {...}}
	msgWithSeq := map[string]interface{}{
		"isReply": seq != 0,
		"seq":     seq,
		"msgID":   msgID,
		"data":    msg,
	}

	data, err := json.Marshal(msgWithSeq)
	return [][]byte{data}, err
}

// MarshalWithSeq 序列化带序列号的消息
// isReply: 是否是回复消息
// seq: 序列号，如果是回复消息则使用客户端发送的序列号，否则为0
func (p *Processor) MarshalWithSeq(msg interface{}, isReply bool, seq uint32) ([][]byte, error) {
	msgType := reflect.TypeOf(msg)
	if msgType == nil || msgType.Kind() != reflect.Ptr {
		return nil, errors.New("json message pointer required")
	}
	msgID := msgType.Elem().Name()
	if _, ok := p.msgInfo[msgID]; !ok {
		return nil, fmt.Errorf("message %v not registered", msgID)
	}

	// 构建新的消息格式: {"isReply": bool, "seq": number, "msgID": string, "data": {...}}
	msgWithSeq := map[string]interface{}{
		"isReply": isReply,
		"seq":     seq,
		"msgID":   msgID,
		"data":    msg,
	}

	data, err := json.Marshal(msgWithSeq)
	return [][]byte{data}, err
}
