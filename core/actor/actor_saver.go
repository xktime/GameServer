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
func cleanupTypeCache(activeKeys map[string]interface{}) {
	// 计算当前缓存大小
	cacheSize := 0
	typeCache.Range(func(_, _ interface{}) bool {
		cacheSize++
		return true
	})

	log.Debug("当前类型缓存大小: %d, 阈值: %d", cacheSize, maxTypeCacheSize)

	// 如果缓存大小超过阈值，清理部分条目
	if cacheSize > maxTypeCacheSize {
		log.Debug("开始清理类型缓存，当前大小: %d", cacheSize)
		deleteCount := 0
		targetSize := maxTypeCacheSize / 2

		// 清理到目标大小，优先清理不活跃的
		typeCache.Range(func(key, _ interface{}) bool {
			if deleteCount < (cacheSize - targetSize) {
				keyStr, ok := key.(string)
				if _, exists := activeKeys[keyStr]; ok && !exists {
					// 删除不活跃的Actor类型缓存
					typeCache.Delete(key)
					deleteCount++
					log.Debug("清理不活跃的Actor类型缓存: %s", keyStr)
				}
				return true
			}
			return false
		})
		log.Debug("已清理 %d 个缓存条目，目标大小: %d", deleteCount, targetSize)
	}
}

// ForceCleanupTypeCache 强制清理类型缓存，清理所有不活跃的Actor类型缓存
func ForceCleanupTypeCache() {
	log.Debug("强制清理类型缓存")

	// 获取当前活跃的Actor keys
	activeKeys := make(map[string]bool)
	actorFactory.mu.RLock()
	for key := range actorFactory.actors {
		activeKeys[key] = true
	}
	actorFactory.mu.RUnlock()

	// 清理所有不活跃的缓存
	deleteCount := 0
	typeCache.Range(func(key, _ interface{}) bool {
		keyStr, ok := key.(string)
		if ok && !activeKeys[keyStr] {
			typeCache.Delete(key)
			deleteCount++
			log.Debug("强制清理不活跃的Actor类型缓存: %s", keyStr)
		}
		return true
	})

	log.Debug("强制清理完成，共清理 %d 个不活跃的缓存条目", deleteCount)
}

// cleanupActorTypeCache 清理指定Actor的类型缓存
func cleanupActorTypeCache(actorKey string) {
	typeCache.Delete(actorKey)
	log.Debug("已清理Actor类型缓存: %s", actorKey)
}

// SaveAllActorData 保存所有实现了ActorData接口的Actor
// 使用快照方式避免长时间加锁，容忍部分数据可能未保存
func SaveAllActorData() {
	// 创建actors的快照，最小化锁的持有时间
	actorsSnapshot := make(map[string]interface{}, len(actorFactory.actors))
	actorFactory.mu.RLock()
	{
		// 创建副本，同时保存key和meta
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

	// 在保存数据后清理缓存，清理掉不再需要的类型缓存
	cleanupTypeCache(actorsSnapshot)
}

func saveMeta(meta interface{}) {
	actorField := getActorByReflect(meta)
	if actorField != nil {
		if data, ok := actorField.(mongodb.PersistData); ok {
			_, err := mongodb.Save(data)
			if err != nil {
				log.Error("保存ActorMeta失败: %v", err)
			}
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
func batchSaveActorData(actorType reflect.Type, dataList []mongodb.PersistData) {
	// 创建一个空的接口切片用于存储数据

	// 使用接口切片并依赖MongoDB驱动的处理
	result, err := mongodb.BulkSave(dataList)

	// 处理结果
	if err != nil {
		return
	}
	log.Debug("批量保存%s类型Actor完成: UpsertedCount%d个, ModifiedCount%d个", actorType.Name(), result.UpsertedCount, result.ModifiedCount)
}
