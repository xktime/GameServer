package models

// MsgWithSeq 带序列号的消息结构
type MsgWithSeq struct {
	IsReply bool        // 是否是回复消息
	Seq     uint32      // 序列号
	MsgID   uint32      // 消息ID
	MsgData interface{} // 消息数据
}
