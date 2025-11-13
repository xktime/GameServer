package utils

import (
	"math/rand"
)

func RandByArray[T any](array []T) int32 {
	result := RandByArrayCount(array, 1)
	if len(result) == 0 {
		return -1
	}
	return result[0]
}

func RandByArrayCount[T any](array []T, count int32) []int32 {
	if len(array) == 0 || count <= 0 {
		return []int32{}
	}

	// 如果请求数量大于等于数组长度，返回所有下标
	if int32(len(array)) <= count {
		result := make([]int32, len(array))
		for i := range array {
			result[i] = int32(i)
		}
		return result
	}

	// 使用Fisher-Yates洗牌算法的变种来高效选择不重复的下标
	indices := make([]int32, len(array))
	for i := range indices {
		indices[i] = int32(i)
	}

	result := make([]int32, 0, count)
	for i := 0; i < int(count); i++ {
		// 从剩余的下标中随机选择一个
		j := rand.Intn(len(indices)-i) + i
		// 交换到前面
		indices[i], indices[j] = indices[j], indices[i]
		result = append(result, indices[i])
	}

	return result
}

func RandWeight(weight map[int32]int32) int32 {
	result := RandWeightByCount(weight, 1)
	if len(result) == 0 {
		return -1
	}
	return result[0]
}

// RandWeight 根据权重随机选择
func RandWeightByCount(weight map[int32]int32, count int32) []int32 {
	if len(weight) == 0 || count <= 0 {
		return []int32{}
	}

	// 先把所有key和累加权重准备好
	type kv struct {
		Key    int32
		Weight int32
	}
	kvs := make([]kv, 0, len(weight))
	var totalWeight int32 = 0
	for k, w := range weight {
		if w > 0 {
			kvs = append(kvs, kv{Key: k, Weight: w})
			totalWeight += w
		}
	}
	if totalWeight == 0 {
		return []int32{}
	}

	// 如果count大于可选项数量，最多返回所有key
	if int32(len(kvs)) < count {
		count = int32(len(kvs))
	}

	result := make([]int32, 0, count)
	used := make(map[int32]struct{})

	for int32(len(result)) < count {
		r := rand.Int31n(totalWeight)
		acc := int32(0)
		for i, item := range kvs {
			acc += item.Weight
			if r < acc {
				// 避免重复
				if _, ok := used[item.Key]; !ok {
					result = append(result, item.Key)
					used[item.Key] = struct{}{}
					// 选中后从kvs和totalWeight中移除
					totalWeight -= item.Weight
					kvs = append(kvs[:i], kvs[i+1:]...)
				}
				break
			}
		}
		// 如果kvs被清空，提前退出
		if len(kvs) == 0 {
			break
		}
	}
	return result
}
