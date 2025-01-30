package lcache

import (
	"sync"
	"time"

	"github.com/juguagua/lcache/store"
)

type cache struct {
	lock       sync.RWMutex
	c          store.Store
	cacheBytes int64 // 最大缓存大小
}

// lruCacheLazyLoadIfNeed 在需要时初始化缓存
func (cache *cache) lazyLoadIfNeed() {
	if cache.c == nil {
		cache.lock.Lock()
		defer cache.lock.Unlock()
		if cache.c == nil {
			// 使用 Store 接口初始化缓存，通过配置来选择具体策略
			cache.c = store.NewStore(store.LRU, store.Options{
				MaxBytes: cache.cacheBytes,
			})
		}
	}
}

// add 向缓存中添加一个 key-value 对
func (cache *cache) add(key string, value ByteView) {
	// 延迟加载缓存
	cache.lazyLoadIfNeed()
	// 调用 Store 接口的 Set 方法
	cache.c.Set(key, value)
}

// get 从缓存中获取值
func (cache *cache) get(key string) (value ByteView, ok bool) {
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

// addWithExpiration 向缓存中添加一个带过期时间的 key-value 对
func (cache *cache) addWithExpiration(key string, value ByteView, expirationTime time.Time) {
	// 延迟加载缓存
	cache.lazyLoadIfNeed()
	// 调用 Store 接口的 SetWithExpiration 方法
	cache.c.SetWithExpiration(key, value, time.Until(expirationTime))
}

// delete 从缓存中删除一个 key
func (cache *cache) delete(key string) bool {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cache.c == nil {
		return false
	}
	// 调用 Store 接口的 Delete 方法
	return cache.c.Delete(key)
}

// Clear 清空缓存
func (cache *cache) Clear() {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if cache.c != nil {
		// 调用 Store 接口的 Clear 方法
		cache.c.Clear()
	}
}

// Len 返回缓存的当前存储量
func (cache *cache) Len() int {
	cache.lock.RLock()
	defer cache.lock.RUnlock()
	if cache.c == nil {
		return 0
	}
	// 调用 Store 接口的 Len 方法
	return cache.c.Len()
}
