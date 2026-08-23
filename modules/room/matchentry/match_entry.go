package matchentry

// MatchedTeam is an immutable team snapshot handed from Match to Room.
type MatchedTeam struct {
	TeamID    int64
	PlayerIDs []int64
	IsRobot   bool
}

// Admission is one idempotent request to turn a matched group into a Room.
type Admission struct {
	MatchID string
	Teams   []MatchedTeam
}

// AcceptanceStatus describes the complete outcome set of an admission.
type AcceptanceStatus uint8

const (
	Accepted AcceptanceStatus = iota + 1
	AlreadyAccepted
	Rejected
	Retryable
)

// Acceptance is the observable result of accepting a matched group.
type Acceptance struct {
	Status          AcceptanceStatus
	RoomID          string
	RejectedTeamIDs []int64
}
