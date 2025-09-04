package monitor

import (
	"fmt"
	"sync"
)

type ChannelStats struct {
	CacheHits       int64 // 缓存命中次数
	CacheMisses     int64 // 缓存未命中次数
	ChannelOps      int64 // channel操作次数
	ChannelTimeouts int64 // channel超时次数
	statsMutex      sync.RWMutex
}

// 统计方法
func (s *ChannelStats) IncrementChannelOps() {
	s.statsMutex.Lock()
	s.ChannelOps++
	s.statsMutex.Unlock()
}

func (s *ChannelStats) IncrementChannelTimeouts() {
	s.statsMutex.Lock()
	s.ChannelTimeouts++
	s.statsMutex.Unlock()
}

func (s *ChannelStats) IncrementCacheHits() {
	s.statsMutex.Lock()
	s.CacheHits++
	s.statsMutex.Unlock()
}

func (s *ChannelStats) IncrementCacheMisses() {
	s.statsMutex.Lock()
	s.CacheMisses++
	s.statsMutex.Unlock()
}

// GetPerformanceStats 获取性能统计信息
func (s *ChannelStats) GetPerformanceStats() map[string]interface{} {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	hitRate := float64(0)
	if s.CacheHits+s.CacheMisses > 0 {
		hitRate = float64(s.CacheHits) / float64(s.CacheHits+s.CacheMisses) * 100
	}

	timeoutRate := float64(0)
	if s.ChannelOps > 0 {
		timeoutRate = float64(s.ChannelTimeouts) / float64(s.ChannelOps) * 100
	}

	return map[string]interface{}{
		"cache_hits":       s.CacheHits,
		"cache_misses":     s.CacheMisses,
		"cache_hit_rate":   fmt.Sprintf("%.2f%%", hitRate),
		"channel_ops":      s.ChannelOps,
		"channel_timeouts": s.ChannelTimeouts,
		"timeout_rate":     fmt.Sprintf("%.2f%%", timeoutRate),
	}
}
