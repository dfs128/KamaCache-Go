package store

import "time"

// Value 缓存值接口
type Value interface {
	Len() int // 返回数据大小
}

// Store 缓存接口
type Store interface {
	Get(key string) (Value, bool)
	Set(key string, value Value) error
	SetWithExpiration(key string, value Value, expiration time.Duration) error
	Delete(key string) bool
	Clear()
	Len() int
}

// CacheType 缓存类型
type CacheType string

const (
	LRU CacheType = "lru"
	LFU CacheType = "lfu"
	// todo more
)

// Options 通用缓存配置选项
type Options struct {
	MaxBytes        int64
	CleanupInterval time.Duration
	OnEvicted       func(key string, value Value)
}

// NewStore 创建缓存存储实例
func NewStore(cacheType CacheType, opts Options) Store {
	switch cacheType {
	case LRU:
		return newLRUCache(opts)
	case LFU:
		return nil // todo
	default:
		return newLRUCache(opts)
	}
}
