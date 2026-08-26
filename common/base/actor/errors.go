package actor

import "errors"

var ErrQueueFull = errors.New("actor: queue is full")
var ErrOutcomeUnknown = errors.New("actor: task outcome is unknown")
var ErrCallCycle = errors.New("actor: synchronous call cycle")
var ErrActorStopped = errors.New("actor: actor is stopped")
var ErrStaleActorRef = errors.New("actor: stale actor reference")
var ErrHandlerPanic = errors.New("actor: handler panicked")
var ErrActorQuarantined = errors.New("actor: actor is quarantined")
var ErrForcedStop = errors.New("actor: task was discarded by force stop")
var ErrScopeStopped = errors.New("actor: scope is stopped")
var ErrSystemStopped = errors.New("actor: system is stopped")
