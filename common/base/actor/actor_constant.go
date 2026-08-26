package actor

type ActorGroup string

const (
	User     ActorGroup = "user"
	Login    ActorGroup = "login"
	Recharge ActorGroup = "recharge"
	Player   ActorGroup = "Player"
	Team     ActorGroup = "Team"
	Room     ActorGroup = "Room"
	Rank     ActorGroup = "Rank"
	Match    ActorGroup = "match"
	// 测试用的ActorGroup
	Test1 ActorGroup = "test1"
	Test2 ActorGroup = "test2"
)
