package actor_manager

import (
	"gameserver/common/db/mongodb"
	"gameserver/core/log"
	"reflect"
	"sync"
)

// 类型缓存，用于存储actor key到reflect.Type的映射
var typeCache sync.Map

// 缓存最大条目数阈值
const maxTypeCacheSize = 10000

// 清理缓存的函数，当缓存大小超过阈值时删除部分条目
func cleanupTypeCache() {
	// 计算当前缓存大小
	cacheSize := 0
	typeCache.Range(func(_, _ interface{}) bool {
		cacheSize++
		return true
	})

	// 如果缓存大小超过阈值，清理一半的条目
	if cacheSize > maxTypeCacheSize {
		log.Debug("清理类型缓存，当前大小: %d, 目标大小: %d", cacheSize, maxTypeCacheSize/2)
		deleteCount := 0
		// 随机删除一半的条目
		typeCache.Range(func(key, _ interface{}) bool {
			if deleteCount < cacheSize/2 {
				typeCache.Delete(key)
				deleteCount++
				return true
			}
			return false
		})
		log.Debug("已清理 %d 个缓存条目", deleteCount)
	}
}

// SaveAllActorData 保存所有实现了ActorData接口的Actor
// 使用快照方式避免长时间加锁，容忍部分数据可能未保存
func SaveAllActorData() {
	// 在保存数据前检查并清理缓存
	cleanupTypeCache()
	// 创建actors的快照，最小化锁的持有时间
	var actorsSnapshot map[string]interface{}
	actorFactory.mu.RLock()
	{
		// 创建副本，同时保存key和meta
		actorsSnapshot = make(map[string]interface{}, len(actorFactory.actors))
		for key, meta := range actorFactory.actors {
			actorsSnapshot[key] = meta
		}
	}
	actorFactory.mu.RUnlock()

	// 按类型分组
	typeGroup := make(map[reflect.Type][]mongodb.PersistData)

	// 在快照上进行遍历，无需加锁
	for cacheKey, meta := range actorsSnapshot {

		// 获取Actor字段的值
		actorValue := getActorByReflect(meta)
		if actorValue == nil {
			continue
		}

		// 检查是否实现了persistData接口
		persistData, ok := actorValue.(mongodb.PersistData)
		if !ok {
			continue
		}

		// 从缓存获取类型
		actorTypeObj, found := typeCache.Load(cacheKey)

		var actorType reflect.Type
		// 如果缓存中没有，则反射获取类型并存入缓存
		if !found {
			actorType = reflect.TypeOf(persistData)
			typeCache.Store(cacheKey, actorType)
		} else {
			// 类型断言
			actorType = actorTypeObj.(reflect.Type)
		}

		// 将ActorData实例添加到对应类型的组中
		typeGroup[actorType] = append(typeGroup[actorType], persistData)
	}

	// 对每个类型进行批量保存
	for actorType, dataList := range typeGroup {
		// 调用批量保存方法
		batchSaveActorData(actorType, dataList)
	}

}

func saveMeta(meta interface{}) {
	actorField := getActorByReflect(meta)
	if actorField != nil {
		if data, ok := actorField.(mongodb.PersistData); ok {
			mongodb.Save(data)
		}
	}
}

func getActorByReflect(meta interface{}) interface{} {
	metaValue := reflect.ValueOf(meta)
	if metaValue.Kind() == reflect.Ptr {
		metaValue = metaValue.Elem()
	}
	actorField := metaValue.FieldByName("Actor")
	if actorField.IsValid() {
		return actorField.Interface()
	}
	return nil
}

// batchSaveActorData 批量保存同类型的ActorData
func batchSaveActorData(actorType reflect.Type, dataList []mongodb.PersistData) (int, int) {
	// 创建一个空的接口切片用于存储数据

	// 使用接口切片并依赖MongoDB驱动的处理
	result, err := mongodb.BulkSave(dataList)

	// 处理结果
	if err != nil {
		return 0, len(dataList)
	}

	// 保存成功
	savedCount := int(result.UpsertedCount + result.ModifiedCount)
	failedCount := len(dataList) - savedCount

	log.Debug("批量保存%s类型Actor完成: 成功%d个, 失败%d个", actorType.Name(), savedCount, failedCount)
	return savedCount, failedCount
}
