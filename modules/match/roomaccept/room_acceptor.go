package roomaccept

import "gameserver/modules/room/matchentry"

type Acceptor interface {
	AcceptMatch(admission matchentry.Admission) matchentry.Acceptance
}
