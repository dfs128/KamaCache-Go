package lcache

import (
	"sync"
	"time"

	"github.com/juguagua/lcache/store"
)

type Cache struct {
	lock sync.RWMutex
	c    store.Store
	opts CacheOptions
}

type CacheOptions struct {
	CacheType    store.CacheType
	MaxBytes     int64
	BucketCount  uint16 // 缓存桶数量
	CapPerBucket uint16 // 每个缓存桶的容量
	Level2Cap    uint16 // 二级缓存桶的容量
	CleanupTime  time.Duration
	OnEvicted    func(key string, value store.Value)
}

func NewCache(opts CacheOptions) *Cache {
	return &Cache{
		opts: opts,
	}
}

// lruCacheLazyLoadIfNeed 在需要时初始化缓存
func (cache *Cache) lazyLoadIfNeed() {
	if cache.c == nil {
		cache.lock.Lock()
		defer cache.lock.Unlock()
		if cache.c == nil {
			opts := store.NewOptions()
			cache.c = store.NewStore(cache.opts.CacheType, opts)
		}
	}
}

// Add 向缓存中添加一个 key-value 对
func (cache *Cache) Add(key string, value ByteView) {
	// 延迟加载缓存
	cache.lazyLoadIfNeed()
	// 调用 Store 接口的 Set 方法
	cache.c.Set(key, value)
}

// Get 从缓存中获取值
func (cache *Cache) get(key string) (value ByteView, ok bool) {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	if cache.c == nil {
		return
	}
	// 调用 Store 接口的 Get 方法
	if v, found := cache.c.Get(key); found {
		return v.(ByteView), true
	}
	return
}

// AddWithExpiration 向缓存中添加一个带过期时间的 key-value 对
func (cache *Cache) AddWithExpiration(key string, value ByteView, expirationTime time.Time) {
	// 延迟加载缓存
	cache.lazyLoadIfNeed()
	// 调用 Store 接口的 SetWithExpiration 方法
	cache.c.SetWithExpiration(key, value, time.Until(expirationTime))
}

// delete 从缓存中删除一个 key
func (cache *Cache) Delete(key string) bool {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cache.c == nil {
		return false
	}
	// 调用 Store 接口的 Delete 方法
	return cache.c.Delete(key)
}

// Clear 清空缓存
func (cache *Cache) Clear() {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cache.c != nil {
		// 调用 Store 接口的 Clear 方法
		cache.c.Clear()
	}
}

// Len 返回缓存的当前存储量
func (cache *Cache) Len() int {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	if cache.c == nil {
		return 0
	}
	// 调用 Store 接口的 Len 方法
	return cache.c.Len()
}

// Close 关闭缓存，释放资源
func (cache *Cache) Close() {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	if closer, ok := cache.c.(interface{ Close() }); ok && closer != nil {
		closer.Close()
	}
	cache.c = nil
}
