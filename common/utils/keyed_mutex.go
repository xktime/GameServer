package utils

import (
	"context"
	"fmt"
	"sync"
)

// KeyedMutex 按键串行执行操作，等待锁时响应上下文取消。零值可直接使用。
// 无持有者和等待者的键会被移除；同一实例不可在首次使用后复制。
type KeyedMutex[K comparable] struct {
	mu      sync.Mutex
	entries map[K]*keyedMutexEntry
}

type keyedMutexEntry struct {
	token chan struct{}
	users int
}

// Lock 返回本次持锁对应的释放函数；成功后必须且只能调用一次。
func (m *KeyedMutex[K]) Lock(ctx context.Context, key K) (func(), error) {
	if ctx == nil {
		return nil, fmt.Errorf("keyed mutex: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.entries == nil {
		m.entries = make(map[K]*keyedMutexEntry)
	}
	entry := m.entries[key]
	if entry == nil {
		entry = &keyedMutexEntry{token: make(chan struct{}, 1)}
		m.entries[key] = entry
	}
	entry.users++
	m.mu.Unlock()

	unlock := func() {
		<-entry.token
		m.release(key, entry)
	}
	select {
	case entry.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			unlock()
			return nil, err
		}
		return unlock, nil
	case <-ctx.Done():
		m.release(key, entry)
		return nil, ctx.Err()
	}
}

func (m *KeyedMutex[K]) release(key K, entry *keyedMutexEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.users--
	if entry.users == 0 {
		delete(m.entries, key)
	}
}
