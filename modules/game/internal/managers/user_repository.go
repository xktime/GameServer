package managers

import (
	"gameserver/common/db/mongodb"
	"gameserver/common/models"

	"go.mongodb.org/mongo-driver/bson"
)

type userRepository interface {
	FindByLogin(openID string, serverID int32) (*models.User, error)
	Save(user *models.User) error
}

type mongoUserRepository struct{}

func (mongoUserRepository) FindByLogin(openID string, serverID int32) (*models.User, error) {
	return mongodb.FindOne[models.User](bson.M{"OpenId": openID, "ServerId": serverID})
}

func (mongoUserRepository) Save(user *models.User) error {
	_, err := mongodb.Save(user)
	return err
}
