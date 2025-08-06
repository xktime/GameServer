package actor_manager

import (
	"gameserver/core/log"
	"reflect"
)

// saveAllActorData 保存所有实现了ActorData接口的Actor
// 使用快照方式避免长时间加锁，容忍部分数据可能未保存
func SaveAllActorData() {
	// 创建actors的快照，最小化锁的持有时间
	var actorsSnapshot []interface{}
	actorFactory.mu.RLock()
	{
		// 创建副本
		actorsSnapshot = make([]interface{}, 0, len(actorFactory.actors))
		for _, meta := range actorFactory.actors {
			actorsSnapshot = append(actorsSnapshot, meta)
		}
	}
	actorFactory.mu.RUnlock()

	var savedCount int
	var failedCount int

	// 在快照上进行遍历，无需加锁
	for _, meta := range actorsSnapshot {
		// 使用反射获取Actor字段的值
		metaValue := reflect.ValueOf(meta)
		if metaValue.Kind() == reflect.Ptr {
			metaValue = metaValue.Elem()
		}

		// 查找Actor字段
		actorField := metaValue.FieldByName("Actor")
		if !actorField.IsValid() {
			continue
		}

		// 获取Actor字段的值
		actorValue := actorField.Interface()

		// todo 批量存储
		// 检查是否实现了ActorData接口
		if data, ok := actorValue.(ActorData); ok {
			// 调用Save方法
			if err := data.Save(); err != nil {
				// 获取Actor的ID
				idField := metaValue.FieldByName("ID")
				var id string
				if idField.IsValid() {
					id = idField.String()
				} else {
					id = "unknown"
				}

				log.Error("保存Actor失败: ID=%s, 错误: %v", id, err)
				failedCount++
			} else {
				savedCount++
			}
		}
	}

	log.Debug("Actor保存完成: 成功%d个, 失败%d个", savedCount, failedCount)
}
