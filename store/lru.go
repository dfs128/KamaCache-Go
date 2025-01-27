package store

import (
	"container/list"
	"sync"
	"time"
)

// lruCache LRU缓存实现
type lruCache struct {
	mu              sync.RWMutex
	list            *list.List
	items           map[string]*list.Element
	expires         map[string]time.Time
	maxBytes        int64
	usedBytes       int64
	onEvicted       func(key string, value Value)
	cleanupInterval time.Duration
}

type lruEntry struct {
	key   string
	value Value
}

// newLRUCache 创建LRU缓存实例
func newLRUCache(opts Options) *lruCache {
	c := &lruCache{
		list:            list.New(),
		items:           make(map[string]*list.Element),
		expires:         make(map[string]time.Time),
		maxBytes:        opts.MaxBytes,
		onEvicted:       opts.OnEvicted,
		cleanupInterval: opts.CleanupInterval,
	}

	if c.cleanupInterval <= 0 {
		c.cleanupInterval = time.Minute // 默认清理间隔
	}

	go c.cleanupLoop()
	return c
}

// Get 实现Store接口
func (c *lruCache) Get(key string) (Value, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if elem, ok := c.items[key]; ok {
		if expTime, hasExp := c.expires[key]; hasExp && time.Now().After(expTime) {
			return nil, false
		}
		c.list.MoveToBack(elem)
		return elem.Value.(*lruEntry).value, true
	}
	return nil, false
}

// Set 实现Store接口
func (c *lruCache) Set(key string, value Value) error {
	return c.SetWithExpiration(key, value, 0)
}

// SetWithExpiration 实现Store接口
func (c *lruCache) SetWithExpiration(key string, value Value, expiration time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.usedBytes += int64(value.Len() - elem.Value.(*lruEntry).value.Len())
		elem.Value.(*lruEntry).value = value
		c.list.MoveToBack(elem)
	} else {
		elem := c.list.PushBack(&lruEntry{key: key, value: value})
		c.items[key] = elem
		c.usedBytes += int64(len(key) + value.Len())
	}

	if expiration > 0 {
		c.expires[key] = time.Now().Add(expiration)
	} else {
		delete(c.expires, key)
	}

	c.evict()
	return nil
}

// Delete 实现Store接口
func (c *lruCache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
		return true
	}
	return false
}

// Clear 实现Store接口
func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.list.Init()
	c.items = make(map[string]*list.Element)
	c.expires = make(map[string]time.Time)
	c.usedBytes = 0
}

// Len 实现Store接口
func (c *lruCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.list.Len()
}

// removeElement 删除缓存元素
func (c *lruCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*lruEntry)
	c.list.Remove(elem)
	delete(c.items, entry.key)
	delete(c.expires, entry.key)
	c.usedBytes -= int64(len(entry.key) + entry.value.Len())

	if c.onEvicted != nil {
		c.onEvicted(entry.key, entry.value)
	}
}

// evict 清理过期和超出内存限制的缓存
func (c *lruCache) evict() {
	now := time.Now()
	for key, expTime := range c.expires {
		if now.After(expTime) {
			if elem, ok := c.items[key]; ok {
				c.removeElement(elem)
			}
		}
	}

	for c.maxBytes > 0 && c.usedBytes > c.maxBytes {
		elem := c.list.Front()
		if elem != nil {
			c.removeElement(elem)
		}
	}
}

// cleanupLoop 定期清理过期缓存
func (c *lruCache) cleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		c.evict()
		c.mu.Unlock()
	}
}
