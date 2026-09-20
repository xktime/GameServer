package actor

// BaseActor 提供通用的Actor基础实现
type BaseActor struct {
	ref ActorRef
}

// ActorBase 向运行时提供内嵌的 Actor 元数据。
func (b *BaseActor) ActorBase() *BaseActor {
	return b
}

// Ref 返回由 ActorSystem 绑定的不可变引用。
func (b *BaseActor) Ref() ActorRef {
	return b.ref
}
