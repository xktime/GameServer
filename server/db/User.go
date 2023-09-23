package db

import "go.mongodb.org/mongo-driver/bson"

type User struct {
	ServerId int
	Account  int
}

func FindByAccount(account int) (User, error) {
	var result User
	err := GetMongoClient().FindOne("game", "users", bson.D{{"account", account}}).Decode(&result)
	return result, err
}

func RegisterUser(ServerId int, account int) error {
	_, err := GetMongoClient().InsertOne("game", "users", &User{ServerId, account})
	if err != nil {
		return err
	}
	return nil
}
