package db

type DB struct {
	MongoClient MongoClient `mapstructure:"mongo"`
}
