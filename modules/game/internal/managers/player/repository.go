package player

import "gameserver/common/db/mongodb"

type playerRepository interface {
	FindByID(playerID int64) (*Player, error)
	Save(player *Player) error
}

type mongoPlayerRepository struct{}

func (mongoPlayerRepository) FindByID(playerID int64) (*Player, error) {
	return mongodb.FindOneById[Player](playerID)
}

func (mongoPlayerRepository) Save(player *Player) error {
	_, err := mongodb.Save(player)
	return err
}
