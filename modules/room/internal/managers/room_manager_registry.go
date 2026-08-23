package managers

import (
	"gameserver/modules/room/participant"
	"sync"
	"time"
)

type roomManagerFactory func(
	participant.TeamRoomProjection,
	participant.PlayerMessenger,
	func() time.Time,
	func() string,
) *RoomManager

type roomManagerRegistry struct {
	mu      sync.Mutex
	started bool
	manager *RoomManager
	failure any
}

var (
	roomManagerRegistration                        = &roomManagerRegistry{}
	createRegisteredRoomManager roomManagerFactory = NewRoomManager
)

func (r *roomManagerRegistry) register(create func() *RoomManager) (manager *RoomManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		panic("room: RegisterRoomManager called more than once")
	}
	r.started = true

	defer func() {
		if failure := recover(); failure != nil {
			r.failure = failure
			panic(failure)
		}
	}()
	manager = create()
	r.manager = manager
	return manager
}

func (r *roomManagerRegistry) get() *RoomManager {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.started {
		panic("room: GetRoomManager called before RegisterRoomManager")
	}
	if r.failure != nil {
		panic(r.failure)
	}
	return r.manager
}

func RegisterRoomManager(
	projection participant.TeamRoomProjection,
	messenger participant.PlayerMessenger,
	now func() time.Time,
	newRoomID func() string,
) *RoomManager {
	return roomManagerRegistration.register(func() *RoomManager {
		if projection == nil {
			panic("room: RegisterRoomManager requires TeamRoomProjection")
		}
		if messenger == nil {
			panic("room: RegisterRoomManager requires PlayerMessenger")
		}
		if now == nil {
			panic("room: RegisterRoomManager requires Clock")
		}
		if newRoomID == nil {
			panic("room: RegisterRoomManager requires RoomID generator")
		}
		return createRegisteredRoomManager(projection, messenger, now, newRoomID)
	})
}

func GetRoomManager() *RoomManager {
	return roomManagerRegistration.get()
}
