package actor

import "errors"

var ErrQueueFull = errors.New("actor: queue is full")
var ErrCallCycle = errors.New("actor: synchronous call cycle")
var ErrActorStopped = errors.New("actor: actor is stopped")
var ErrActorNotFound = errors.New("actor: actor does not exist")
var ErrActorStopping = errors.New("actor: actor is stopping")
var ErrStopFailed = errors.New("actor: stop failed; persistence must be retried")
var ErrStaleActorRef = errors.New("actor: stale actor reference")
var ErrHandlerPanic = errors.New("actor: handler panicked")
var ErrActorQuarantined = errors.New("actor: actor is quarantined")
var ErrActorNotQuarantined = errors.New("actor: actor is not quarantined")
var ErrStaleQuarantineTarget = errors.New("actor: stale quarantine target")
var ErrForcedStop = errors.New("actor: task was discarded by force stop")
var ErrScopeStopped = errors.New("actor: scope is stopped")
var ErrSystemStopped = errors.New("actor: system is stopped")
