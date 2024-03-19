package db

import "go.mongodb.org/mongo-driver/bson"

type User struct {
	ServerId uint32
	Account  string
}

func FindByAccount(account string) (User, error) {
	var result User
	err := GetMongoClient().FindOne("game", "users", bson.D{{"account", account}}).Decode(&result)
	return result, err
}

func RegisterUser(ServerId uint32, account string) error {
	_, err := GetMongoClient().InsertOne("game", "users", &User{ServerId, account})
	if err != nil {
		return err
	}
	return nil
}
