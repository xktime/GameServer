package actor

import (
	"fmt"
	"gameserver/common/db/mongodb"
	"gameserver/core/log"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// TypeCache 类型缓存结构，提供线程安全的类型缓存
type TypeCache struct {
	cache     sync.Map
	size      int64
	maxSize   int64
	mu        sync.RWMutex
	lastClean time.Time
}

// 全局类型缓存实例
var globalTypeCache = &TypeCache{
	maxSize: 10000,
}

// 保存统计信息
type SaveStats struct {
	TotalActors  int64
	SavedActors  int64
	FailedActors int64
	BatchCount   int64
	SaveDuration time.Duration
	LastSaveTime time.Time
}

var saveStats SaveStats
var statsMu sync.RWMutex

// 清理间隔
const cleanupInterval = 5 * time.Minute

// GetType 获取类型，如果不存在则创建并缓存
func (tc *TypeCache) GetType(key string, data interface{}) reflect.Type {
	// 先尝试从缓存获取
	if cached, ok := tc.cache.Load(key); ok {
		return cached.(reflect.Type)
	}

	// 缓存未命中，创建新类型
	actorType := reflect.TypeOf(data)
	tc.cache.Store(key, actorType)
	atomic.AddInt64(&tc.size, 1)

	// 检查是否需要清理
	tc.mu.RLock()
	lastClean := tc.lastClean
	tc.mu.RUnlock()

	if time.Since(lastClean) > cleanupInterval {
		tc.cleanupIfNeeded()
	}

	return actorType
}

// cleanupIfNeeded 在需要时清理缓存
func (tc *TypeCache) cleanupIfNeeded() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 双重检查
	if time.Since(tc.lastClean) <= cleanupInterval {
		return
	}

	currentSize := atomic.LoadInt64(&tc.size)
	if currentSize <= tc.maxSize {
		tc.lastClean = time.Now()
		return
	}

	log.Debug("开始清理类型缓存，当前大小: %d, 阈值: %d", currentSize, tc.maxSize)

	// 获取活跃的Actor keys
	activeKeys := make(map[string]bool)
	globalActorManager.mu.RLock()
	for key := range globalActorManager.taskHandlers {
		activeKeys[key] = true
	}
	globalActorManager.mu.RUnlock()

	// 清理不活跃的缓存
	deleteCount := 0
	targetSize := tc.maxSize / 2

	tc.cache.Range(func(key, value interface{}) bool {
		if deleteCount >= (int(currentSize) - int(targetSize)) {
			return false
		}

		keyStr, ok := key.(string)
		if ok && !activeKeys[keyStr] {
			tc.cache.Delete(key)
			atomic.AddInt64(&tc.size, -1)
			deleteCount++
		}
		return true
	})

	tc.lastClean = time.Now()
	log.Debug("已清理 %d 个缓存条目，目标大小: %d", deleteCount, targetSize)
}

// ForceCleanupTypeCache 强制清理类型缓存，清理所有不活跃的Actor类型缓存
func ForceCleanupTypeCache() {
	log.Debug("强制清理类型缓存")

	// 获取当前活跃的Actor keys
	activeKeys := make(map[string]bool)
	globalActorManager.mu.RLock()
	for key := range globalActorManager.taskHandlers {
		activeKeys[key] = true
	}
	globalActorManager.mu.RUnlock()

	// 清理所有不活跃的缓存
	deleteCount := 0
	globalTypeCache.cache.Range(func(key, _ interface{}) bool {
		keyStr, ok := key.(string)
		if ok && !activeKeys[keyStr] {
			globalTypeCache.cache.Delete(key)
			atomic.AddInt64(&globalTypeCache.size, -1)
			deleteCount++
			log.Debug("强制清理不活跃的Actor类型缓存: %s", keyStr)
		}
		return true
	})

	log.Debug("强制清理完成，共清理 %d 个不活跃的缓存条目", deleteCount)
}

// cleanupActorTypeCache 清理指定Actor的类型缓存
func cleanupActorTypeCache(actorKey string) {
	globalTypeCache.cache.Delete(actorKey)
	atomic.AddInt64(&globalTypeCache.size, -1)
	log.Debug("已清理Actor类型缓存: %s", actorKey)
}

// GetSaveStats 获取保存统计信息
func GetSaveStats() SaveStats {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return saveStats
}

// ResetSaveStats 重置保存统计信息
func ResetSaveStats() {
	statsMu.Lock()
	defer statsMu.Unlock()
	saveStats = SaveStats{}
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
		for _, a := range taskHandler.actors {
			// 检查是否实现了persistData接口
			persistData, ok := a.(mongodb.PersistData)
			if !ok {
				continue
			}

			// 使用优化的类型缓存
			actorType := globalTypeCache.GetType(cacheKey, persistData)

			// 将ActorData实例添加到对应类型的组中
			typeGroup[actorType] = append(typeGroup[actorType], persistData)
			totalActors++
		}
	}

	// 更新统计信息
	statsMu.Lock()
	saveStats.TotalActors = int64(totalActors)
	saveStats.BatchCount = int64(len(typeGroup))
	saveStats.LastSaveTime = startTime
	statsMu.Unlock()

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
	statsMu.Lock()
	saveStats.SavedActors = int64(savedCount)
	saveStats.FailedActors = int64(failedCount)
	saveStats.SaveDuration = duration
	statsMu.Unlock()

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
