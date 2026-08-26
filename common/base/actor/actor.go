package actor

// BaseActor 提供通用的Actor基础实现
type BaseActor struct {
	ref ActorRef
}

// ActorBase exposes the embedded actor metadata to the runtime.
func (b *BaseActor) ActorBase() *BaseActor {
	return b
}

// Ref returns the immutable reference bound by ActorSystem.
func (b *BaseActor) Ref() ActorRef {
	return b.ref
}
