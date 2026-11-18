package roomaccept

import (
	"context"
	"gameserver/modules/room/matchentry"
)

type Acceptor interface {
	AcceptMatch(ctx context.Context, admission matchentry.Admission) matchentry.Acceptance
}
