package managers

import (
	"context"
	"gameserver/common/db/mongodb"
	"gameserver/common/models"

	"go.mongodb.org/mongo-driver/bson"
)

type userRepository interface {
	FindByLogin(ctx context.Context, openID string, serverID int32) (*models.User, error)
	Save(ctx context.Context, user *models.User) error
}

type mongoUserRepository struct{}

func (mongoUserRepository) FindByLogin(ctx context.Context, openID string, serverID int32) (*models.User, error) {
	return mongodb.FindOneContext[models.User](ctx, bson.M{"OpenId": openID, "ServerId": serverID})
}

func (mongoUserRepository) Save(ctx context.Context, user *models.User) error {
	_, err := mongodb.SaveContext(ctx, user)
	return err
}
