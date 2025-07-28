package mongodb

import (
	"context"
	"errors"
	"fmt"
	"gameserver/conf"
	"reflect"
	"time"

	"gameserver/core/log"

	"go.mongodb.org/mongo-driver/bson"
	mongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	client   *mongo.Client
	database *mongo.Database
}

var mongoInstance *Mongo

// 初始化连接
func Init(uri, dbName string, minPoolSize, maxPoolSize uint64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clientOptions := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(maxPoolSize). // 最大连接数
		SetMinPoolSize(minPoolSize). // 最小连接数
		SetMaxConnIdleTime(5 * time.Minute).
		SetServerSelectionTimeout(5 * time.Second)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal("init mongodb failed: %v", err)
		return err
	}
	if err = client.Ping(ctx, nil); err != nil {
		log.Fatal("init mongodb failed: %v", err)
		return err
	}
	mongoInstance = &Mongo{
		client:   client,
		database: client.Database(dbName),
	}
	log.Release("mongodb init dbName: %s, minPoolSize: %d, maxPoolSize: %d", dbName, minPoolSize, maxPoolSize)
	return nil
}

// 查询单条
func FindOne[T any](filter interface{}) (*T, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result T
	err := mongoInstance.getCollection(collection).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func FindOneById[T any](id interface{}) (*T, error) {
	return FindOne[T](bson.M{"_id": id})
}

// 查询多条
func FindAll[T any](filter bson.M) ([]T, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := mongoInstance.getCollection(collection).Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var results []T
	for cur.Next(ctx) {
		var elem T
		if err := cur.Decode(&elem); err != nil {
			return nil, err
		}
		results = append(results, elem)
	}
	return results, cur.Err()
}

// 删除
func DeleteByID[T any](id interface{}) (*mongo.DeleteResult, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return mongoInstance.getCollection(collection).DeleteOne(ctx, bson.M{"_id": id})
}

func Save(doc any) (*mongo.UpdateResult, error) {
	id, err := getID(doc)
	if err != nil {
		return nil, err
	}
	collection := getCollectionName(doc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := options.Replace().SetUpsert(true)
	return mongoInstance.getCollection(collection).ReplaceOne(ctx, bson.M{"_id": id}, doc, opts)
}

// 获取集合
func (m *Mongo) getCollection(collection string) *mongo.Collection {
	return mongoInstance.database.Collection(collection)
}

// 获取泛型T对应的collection名称
func getCollectionNameByType[T any]() string {
	var t T
	return getCollectionName(t)
}

func getCollectionName(t any) string {
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}

func getID(doc any) (interface{}, error) {
	val := reflect.ValueOf(doc)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, errors.New("doc 必须是结构体或结构体指针")
	}

	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		bsonTag := field.Tag.Get("bson")
		if bsonTag == "_id" || field.Name == "ID" || field.Name == "Id" {
			return val.Field(i).Interface(), nil
		}
	}
	return nil, errors.New("doc 缺少 _id 字段")
}

func CreateIndexes(configs conf.MongoIndexConfigs) error {
	for _, idxConf := range configs.Indexes {
		coll := mongoInstance.database.Collection(idxConf.Collection)
		// 1. 获取现有索引
		cursor, err := coll.Indexes().List(context.Background())
		if err != nil {
			return err
		}
		var existingIndexes []bson.M
		if err = cursor.All(context.Background(), &existingIndexes); err != nil {
			return err
		}

		// 2. 构建配置中索引的唯一标识（key+unique）
		type indexSignature struct {
			Keys   bson.D
			Unique bool
		}
		configIndexMap := map[string]indexSignature{}
		for _, create := range idxConf.Create {
			keys := bson.D{}
			for k, v := range create.Keys {
				keys = append(keys, bson.E{Key: k, Value: v})
			}
			sig := indexSignature{Keys: keys, Unique: create.Unique}
			// 用 keys 的 JSON 作为 map key
			keyBytes, _ := bson.MarshalExtJSON(keys, false, false)
			configIndexMap[string(keyBytes)+fmt.Sprint(create.Unique)] = sig

			// 创建索引（如果不存在）
			opts := options.Index().SetUnique(create.Unique)
			_, err := coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
				Keys:    keys,
				Options: opts,
			})
			if err != nil {
				log.Error("创建索引失败: %v", err)
				continue
			}
		}

		// 3. 删除配置中没有的索引（_id_ 除外）
		for _, idx := range existingIndexes {
			name, _ := idx["name"].(string)
			if name == "_id_" {
				continue
			}
			keys, _ := idx["key"].(bson.M)
			unique, _ := idx["unique"].(bool)
			// 转换为 bson.D
			keysD := bson.D{}
			for k, v := range keys {
				keysD = append(keysD, bson.E{Key: k, Value: v})
			}
			keyBytes, _ := bson.MarshalExtJSON(keysD, false, false)
			mapKey := string(keyBytes) + fmt.Sprint(unique)
			if _, ok := configIndexMap[mapKey]; !ok {
				// 配置中没有，删除
				_, err := coll.Indexes().DropOne(context.Background(), name)
				if err != nil && err != mongo.ErrNilDocument {
					log.Error("删除索引失败: %v", err)
					continue
				}
			}
		}
	}
	return nil
}
