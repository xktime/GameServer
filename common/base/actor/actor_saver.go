package actor

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/core/log"
	"math/rand"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// TypeCache 类型缓存结构，提供线程安全的类型缓存
type TypeCache struct {
	cache   sync.Map
	size    int64
	maxSize int64
}

// 全局类型缓存实例
var globalTypeCache = &TypeCache{
	maxSize: 10000,
}
var statsMu sync.RWMutex

// GetType 获取类型，如果不存在则创建并缓存
func (tc *TypeCache) GetType(name, key string, data interface{}) reflect.Type {
	uniqueKey := fmt.Sprintf("%s_%s", name, key)
	// 先尝试从缓存获取
	if cached, ok := tc.cache.Load(uniqueKey); ok {
		return cached.(reflect.Type)
	}

	// 缓存未命中，创建新类型
	actorType := reflect.TypeOf(data)
	tc.cache.Store(uniqueKey, actorType)
	newSize := atomic.AddInt64(&tc.size, 1)

	// 检查是否需要清理缓存
	if newSize > tc.maxSize {
		tc.cleanupIfNeeded()
	}

	return actorType
}

// cleanupIfNeeded 在需要时清理缓存
func (tc *TypeCache) cleanupIfNeeded() {
	currentSize := atomic.LoadInt64(&tc.size)
	if currentSize <= tc.maxSize {
		return
	}

	log.Debug("开始清理类型缓存，当前大小: %d, 阈值: %d", currentSize, tc.maxSize)

	// 计算需要清理的数量（清理一半）
	targetSize := tc.maxSize / 2
	cleanupCount := int(currentSize - targetSize)

	// 收集所有缓存的键
	var keys []interface{}
	tc.cache.Range(func(key, value interface{}) bool {
		keys = append(keys, key)
		return true
	})

	// 如果键的数量少于需要清理的数量，清理所有
	if len(keys) <= cleanupCount {
		cleanupCount = len(keys)
	}

	// 随机选择要清理的键
	rand.Shuffle(len(keys), func(i, j int) {
		keys[i], keys[j] = keys[j], keys[i]
	})

	// 删除选中的键
	deletedCount := 0
	for i := 0; i < cleanupCount; i++ {
		// 检查键是否存在，然后删除
		if _, exists := tc.cache.Load(keys[i]); exists {
			tc.cache.Delete(keys[i])
			deletedCount++
		}
	}

	// 更新大小计数器
	atomic.AddInt64(&tc.size, -int64(deletedCount))

	log.Debug("类型缓存清理完成: 删除了 %d 个条目，目标大小: %d", deletedCount, targetSize)
}

// ForceCleanupTypeCache 强制清理类型缓存，清理所有不活跃的Actor类型缓存
func ForceCleanupTypeCache() {
	log.Debug("强制清理类型缓存")

	// 收集所有缓存的键
	var keys []interface{}
	globalTypeCache.cache.Range(func(key, value interface{}) bool {
		keys = append(keys, key)
		return true
	})

	// 随机选择一半进行清理
	cleanupCount := len(keys) / 2
	if cleanupCount == 0 && len(keys) > 0 {
		cleanupCount = 1
	}

	// 随机打乱键的顺序
	rand.Shuffle(len(keys), func(i, j int) {
		keys[i], keys[j] = keys[j], keys[i]
	})

	// 删除选中的键
	deletedCount := 0
	for i := 0; i < cleanupCount; i++ {
		if _, exists := globalTypeCache.cache.Load(keys[i]); exists {
			globalTypeCache.cache.Delete(keys[i])
			deletedCount++
		}
	}

	// 更新大小计数器
	atomic.AddInt64(&globalTypeCache.size, -int64(deletedCount))

	log.Debug("强制清理完成，共清理 %d 个缓存条目", deletedCount)
}

// SaveAllActorData 保存所有实现了ActorData接口的Actor
// 使用快照方式避免长时间加锁，容忍部分数据可能未保存
func SaveAllActorData() {
	startTime := time.Now()

	// 创建actors的快照，最小化锁的持有时间
	actorsSnapshot := make(map[string]*TaskHandler)
	globalActorManager.mu.RLock()
	{
		// 创建副本，同时保存key和meta
		for key, meta := range globalActorManager.taskHandlers {
			actorsSnapshot[key] = meta
		}
	}
	globalActorManager.mu.RUnlock()

	// 按类型分组
	typeGroup := make(map[reflect.Type][]mongodb.PersistData)
	totalActors := 0

	// 在快照上进行遍历，无需加锁
	for cacheKey, taskHandler := range actorsSnapshot {
		for name, a := range taskHandler.actors {
			// 检查是否实现了persistData接口
			persistData, ok := a.(mongodb.PersistData)
			if !ok {
				continue
			}

			// 使用优化的类型缓存
			actorType := globalTypeCache.GetType(name, cacheKey, persistData)

			// 将ActorData实例添加到对应类型的组中
			typeGroup[actorType] = append(typeGroup[actorType], persistData)
			totalActors++
		}
	}

	// 对每个类型进行批量保存
	savedCount := 0
	failedCount := 0

	for actorType, dataList := range typeGroup {
		saved, failed := batchSaveActorData(actorType, dataList)
		savedCount += saved
		failedCount += failed
	}

	// 更新最终统计信息
	duration := time.Since(startTime)

	log.Debug("Actor数据保存完成: 总数=%d, 成功=%d, 失败=%d, 批次=%d, 耗时=%v",
		totalActors, savedCount, failedCount, len(typeGroup), duration)

	// 在保存数据后清理缓存
	globalTypeCache.cleanupIfNeeded()
}

// saveMeta 保存单个Actor的元数据
func saveMeta(meta interface{}) error {
	actorField := getActorByReflect(meta)
	if actorField != nil {
		if data, ok := actorField.(mongodb.PersistData); ok {
			_, err := mongodb.Save(data)
			if err != nil {
				log.Error("保存ActorMeta失败: %v", err)
				return err
			}
		}
	}
	return nil
}

// getActorByReflect 通过反射获取Actor字段
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
// 返回成功保存的数量和失败的数量
func batchSaveActorData(actorType reflect.Type, dataList []mongodb.PersistData) (int, int) {
	if len(dataList) == 0 {
		return 0, 0
	}

	// 使用接口切片并依赖MongoDB驱动的处理
	result, err := mongodb.BulkSave(dataList)

	// 处理结果
	if err != nil {
		log.Error("批量保存%s类型Actor失败: %v, 数据量: %d", actorType.Name(), err, len(dataList))
		return 0, len(dataList)
	}

	// 计算成功和失败的数量
	successCount := int(result.UpsertedCount + result.ModifiedCount)
	failedCount := len(dataList) - successCount

	if failedCount > 0 {
		log.Error("批量保存%s类型Actor部分失败: 成功=%d, 失败=%d, 总数=%d",
			actorType.Name(), successCount, failedCount, len(dataList))
	} else {
		log.Debug("批量保存%s类型Actor完成: UpsertedCount=%d, ModifiedCount=%d, 总数=%d",
			actorType.Name(), result.UpsertedCount, result.ModifiedCount, len(dataList))
	}

	return successCount, failedCount
}

// SaveActorDataByType 按类型保存Actor数据
func SaveActorDataByType(actorType reflect.Type) error {
	// 收集指定类型的所有Actor数据
	var dataList []mongodb.PersistData

	globalActorManager.mu.RLock()
	for _, taskHandler := range globalActorManager.taskHandlers {
		for _, a := range taskHandler.actors {
			if reflect.TypeOf(a) == actorType {
				if persistData, ok := a.(mongodb.PersistData); ok {
					dataList = append(dataList, persistData)
				}
			}
		}
	}
	globalActorManager.mu.RUnlock()

	if len(dataList) == 0 {
		log.Debug("没有找到类型为%s的Actor数据", actorType.Name())
		return nil
	}

	// 批量保存
	saved, failed := batchSaveActorData(actorType, dataList)
	if failed > 0 {
		return fmt.Errorf("保存失败: 成功=%d, 失败=%d", saved, failed)
	}

	return nil
}
