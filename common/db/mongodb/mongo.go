package mongodb

import (
	"context"
	"fmt"
	"gameserver/conf"
	"reflect"
	"time"

	"gameserver/core/log"

	"go.mongodb.org/mongo-driver/bson"
	mongo "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// actor里面的数据才会自动保存
// 其他地方需要改了马上存储，否则都需要放到actor里面去修改
type PersistData interface {
	GetPersistId() interface{}
}

// 需要持久化的manager实现这个类
// 因为manager的init是用模板生成的，需要加载的数据和初始化需要实现OnInitData
type PersistManager interface {
	PersistData
	OnInitData()
}

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
func FindOne[T PersistData](filter interface{}) (*T, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result T
	err := mongoInstance.getCollection(collection).FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

func FindOneById[T PersistData](id interface{}) (*T, error) {
	return FindOne[T](bson.M{"_id": id})
}

// 查询多条
func FindAll[T PersistData](filter bson.M) ([]T, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := mongoInstance.getCollection(collection).Find(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
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
func DeleteByID[T PersistData](id interface{}) (*mongo.DeleteResult, error) {
	collection := getCollectionNameByType[T]()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := mongoInstance.getCollection(collection).DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		log.Error("DeleteByID: 在集合 %s 中删除ID为 %v 的文档失败: %v", collection, id, err)
		return result, err
	}
	if result.DeletedCount == 0 {
		log.Debug("DeleteByID: 在集合 %s 中未找到ID为 %v 的文档", collection, id)
	} else {
		log.Debug("DeleteByID: 在集合 %s 中成功删除ID为 %v 的文档", collection, id)
	}
	return result, nil
}

func Save(doc PersistData) (*mongo.UpdateResult, error) {
	if mongoInstance == nil {
		log.Debug("Save: MongoDB未初始化，跳过保存")
		return nil, nil
	}
	id := doc.GetPersistId()
	collection := getCollectionName(doc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := options.Replace().SetUpsert(true)
	result, err := mongoInstance.getCollection(collection).ReplaceOne(ctx, bson.M{"_id": id}, doc, opts)
	if err != nil {
		log.Error("Save: 在集合 %s 中保存ID为 %v 的文档失败: %v", collection, id, err)
		return result, err
	}
	if result.UpsertedCount > 0 {
		log.Debug("Save: 在集合 %s 中成功插入ID为 %v 的文档", collection, id)
	} else if result.ModifiedCount > 0 {
		log.Debug("Save: 在集合 %s 中成功更新ID为 %v 的文档", collection, id)
	} else {
		log.Debug("Save: 在集合 %s 中保存ID为 %v 的文档，但无变化", collection, id)
	}
	return result, nil
}

// BulkSave 批量保存文档
// 使用MongoDB的BulkWrite功能，支持upsert
func BulkSave(docs []PersistData) (*mongo.BulkWriteResult, error) {
	if len(docs) == 0 {
		return nil, nil
	}

	// 获取集合名称
	collection := getCollectionName(docs[0])

	// 创建批量写入模型
	models := make([]mongo.WriteModel, 0, len(docs))

	for _, doc := range docs {
		model := mongo.NewReplaceOneModel().
			SetFilter(bson.M{"_id": doc.GetPersistId()}).
			SetReplacement(doc).SetUpsert(true)

		models = append(models, model)
	}

	// 执行批量写入
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mongoInstance.getCollection(collection).BulkWrite(ctx, models)
	if err != nil {
		return nil, fmt.Errorf("批量保存失败: %w", err)
	}

	log.Debug("集合:%s, 批量保存成功: 插入%d个, 更新%d个, ", collection, result.UpsertedCount, result.ModifiedCount)
	return result, nil
}

// 获取集合
func (m *Mongo) getCollection(collection string) *mongo.Collection {
	return mongoInstance.database.Collection(collection)
}

// 获取泛型T对应的collection名称
func getCollectionNameByType[T PersistData]() string {
	var t T
	return getCollectionName(t)
}

func getCollectionName(t PersistData) string {
	typ := reflect.TypeOf(t)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}

func CreateIndexes(configs conf.MongoIndexConfigs) error {
	for _, idxConf := range configs.Indexes {
		coll := mongoInstance.database.Collection(idxConf.Collection)
		log.Debug("CreateIndexes: 处理集合 %s 的索引配置", idxConf.Collection)
		// 1. 获取现有索引
		cursor, err := coll.Indexes().List(context.Background())
		if err != nil {
			log.Error("CreateIndexes: 获取集合 %s 的索引列表失败: %v", idxConf.Collection, err)
			return err
		}
		var existingIndexes []bson.M
		if err = cursor.All(context.Background(), &existingIndexes); err != nil {
			log.Error("CreateIndexes: 解析集合 %s 的索引列表失败: %v", idxConf.Collection, err)
			return err
		}
		log.Debug("CreateIndexes: 集合 %s 当前有 %d 个索引", idxConf.Collection, len(existingIndexes))

		// 2. 构建配置中索引的唯一标识（key+unique）
		type indexSignature struct {
			Keys   bson.D
			Unique bool
		}
		configIndexMap := map[string]indexSignature{}
		createdCount := 0
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
			indexName, err := coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
				Keys:    keys,
				Options: opts,
			})
			if err != nil {
				log.Error("CreateIndexes: 在集合 %s 中创建索引失败: %v", idxConf.Collection, err)
				continue
			}
			createdCount++
			log.Debug("CreateIndexes: 在集合 %s 中成功创建索引: %s", idxConf.Collection, indexName)
		}
		log.Debug("CreateIndexes: 在集合 %s 中成功创建了 %d 个索引", idxConf.Collection, createdCount)

		// 3. 删除配置中没有的索引（_id_ 除外）
		deletedCount := 0
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
					log.Error("CreateIndexes: 删除集合 %s 的索引 %s 失败: %v", idxConf.Collection, name, err)
					continue
				}
				deletedCount++
				log.Debug("CreateIndexes: 从集合 %s 中删除了未配置的索引: %s", idxConf.Collection, name)
			}
		}
		log.Debug("CreateIndexes: 从集合 %s 中删除了 %d 个未配置的索引", idxConf.Collection, deletedCount)
	}
	log.Debug("CreateIndexes: 索引配置处理完成")
	return nil
}
