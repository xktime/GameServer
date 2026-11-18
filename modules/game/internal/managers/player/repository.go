package player

import (
	"context"
	"gameserver/common/db/mongodb"

	"go.mongodb.org/mongo-driver/bson"
)

type Repository interface {
	FindByID(ctx context.Context, playerID int64) (*Player, error)
	FindByName(ctx context.Context, name string) (*Player, error)
	FindAll(ctx context.Context) ([]Player, error)
	Save(ctx context.Context, player *Player) error
}

type mongoPlayerRepository struct{}

func (mongoPlayerRepository) FindByID(ctx context.Context, playerID int64) (*Player, error) {
	return mongodb.FindOneByIDContext[Player](ctx, playerID)
}

func (mongoPlayerRepository) FindByName(ctx context.Context, name string) (*Player, error) {
	return mongodb.FindOneContext[Player](ctx, bson.M{"player_info.player_name": name})
}

func (mongoPlayerRepository) FindAll(ctx context.Context) ([]Player, error) {
	return mongodb.FindAllContext[Player](ctx, bson.M{})
}

func (mongoPlayerRepository) Save(ctx context.Context, player *Player) error {
	_, err := mongodb.SaveContext(ctx, player)
	return err
}
