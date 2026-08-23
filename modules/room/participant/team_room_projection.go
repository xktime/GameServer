package participant

// DesiredTeamRoom is the latest Room projection desired for one real Team.
// An empty RoomID clears the projection.
type DesiredTeamRoom struct {
	TeamID int64
	RoomID string
}

// TeamRoomProjection applies idempotent Team Room projections.
type TeamRoomProjection interface {
	Apply(desired []DesiredTeamRoom) error
}
