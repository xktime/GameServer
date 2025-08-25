package utils

import (
	"fmt"
	"hash/fnv"
)

// 布隆过滤器实现
type BloomFilter struct {
	bits    []bool
	size    int
	hashNum int
}

func NewBloomFilter(size, hashNum int) *BloomFilter {
	return &BloomFilter{
		bits:    make([]bool, size),
		size:    size,
		hashNum: hashNum,
	}
}

func (bf *BloomFilter) Add(item string) {
	for i := 0; i < bf.hashNum; i++ {
		hash := bf.hash(item, i)
		bf.bits[int(hash)%bf.size] = true
	}
}

func (bf *BloomFilter) Contains(item string) bool {
	for i := 0; i < bf.hashNum; i++ {
		hash := bf.hash(item, i)
		if !bf.bits[int(hash)%bf.size] {
			return false
		}
	}
	return true
}

func (bf *BloomFilter) hash(item string, seed int) uint32 {
	h := fnv.New32a()
	h.Write([]byte(fmt.Sprintf("%s_%d", item, seed)))
	return h.Sum32()
}
