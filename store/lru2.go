package store

import (
	"sync"
	"sync/atomic"
	"time"
)

// 内部时钟，减少 time.Now() 调用造成的 GC 压力
var clock, p, n = time.Now().UnixNano(), uint16(0), uint16(1)

// 返回 clock 变量的当前值。atomic.LoadInt64 是原子操作，用于保证在多线程/协程环境中安全地读取 clock 变量的值
func now() int64 { return atomic.LoadInt64(&clock) }

func init() {
	go func() {
		for {

			atomic.StoreInt64(&clock, time.Now().UnixNano()) // 每秒校准一次
			for i := 0; i < 9; i++ {
				time.Sleep(100 * time.Millisecond)
				atomic.AddInt64(&clock, int64(100*time.Millisecond)) // 保持 clock 在一个精确的时间范围内，同时避免频繁的系统调用
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

func hashBKRD(s string) (hash int32) {
	for i := 0; i < len(s); i++ {
		hash = hash*131 + int32(s[i])
	}

	return hash
}

func maskOfNextPowOf2(cap uint16) uint16 {
	if cap > 0 && cap&(cap-1) == 0 {
		return cap - 1
	}
	cap |= cap >> 1
	cap |= cap >> 2
	cap |= cap >> 4
	return cap | (cap >> 8)
}

type node struct {
	k        string
	v        Value
	expireAt int64 // 过期时间戳，expireAt = 0 表示已删除
}

type cache struct {
	dlnk [][2]uint16
	m    []node
	hmap map[string]uint16
	last uint16 // 最后一个节点元素的索引
}

func create(cap uint16) *cache {
	return &cache{
		dlnk: make([][2]uint16, uint32(cap+1)),
		m:    make([]node, cap),
		hmap: make(map[string]uint16),
		last: 0,
	}
}

func (c *cache) put(key string, val Value, expireAt int64, onEvicted func(string, Value)) int {
	if idx, ok := c.hmap[key]; ok {
		c.m[idx-1].v, c.m[idx-1].expireAt = val, expireAt
		c.adjust(idx, p, n)
		return 0
	}

	if c.last == uint16(cap(c.m)) {
		tail := &c.m[c.dlnk[0][p]-1]
		if onEvicted != nil && (*tail).expireAt > 0 {
			onEvicted((*tail).k, (*tail).v)
		}

		delete(c.hmap, (*tail).k)
		c.hmap[key], (*tail).k, (*tail).v, (*tail).expireAt = c.dlnk[0][p], key, val, expireAt
		c.adjust(c.dlnk[0][p], p, n)

		return 1
	}

	c.last++
	if len(c.hmap) <= 0 {
		c.dlnk[0][p] = c.last
	} else {
		c.dlnk[c.dlnk[0][n]][p] = c.last
	}

	// 初始化新节点并更新链表指针
	c.m[c.last-1].k = key
	c.m[c.last-1].v = val
	c.m[c.last-1].expireAt = expireAt
	c.dlnk[c.last] = [2]uint16{0, c.dlnk[0][n]}
	c.hmap[key] = c.last
	c.dlnk[0][n] = c.last

	return 1
}

func (c *cache) get(key string) (*node, int) {
	if idx, ok := c.hmap[key]; ok {
		c.adjust(idx, p, n)
		return &c.m[idx-1], 0
	}
	return nil, 1
}

func (c *cache) del(key string) (*node, int, int64) {
	if idx, ok := c.hmap[key]; ok && c.m[idx-1].expireAt > 0 {
		e := c.m[idx-1].expireAt
		c.m[idx-1].expireAt = 0
		c.adjust(idx, n, p)
		return &c.m[idx-1], 1, e
	}

	return nil, 0, 0
}

func (c *cache) walk(walker func(key string, value Value, expireAt int64) bool) {
	for idx := c.dlnk[0][n]; idx != 0; idx = c.dlnk[idx][n] {
		if c.m[idx-1].expireAt > 0 && !walker(c.m[idx-1].k, c.m[idx-1].v, c.m[idx-1].expireAt) {
			return
		}
	}
}

func (c *cache) adjust(idx, f, t uint16) {
	if c.dlnk[idx][f] != 0 {
		c.dlnk[c.dlnk[idx][t]][f] = c.dlnk[idx][f]
		c.dlnk[c.dlnk[idx][f]][t] = c.dlnk[idx][t]
		c.dlnk[idx][f] = 0
		c.dlnk[idx][t] = c.dlnk[0][t]
		c.dlnk[c.dlnk[0][t]][f] = idx
		c.dlnk[0][t] = idx
	}
}

type lru2Store struct {
	locks       []sync.Mutex
	caches      [][2]*cache
	onEvicted   func(key string, value Value)
	expirations sync.Map
	cleanupTick *time.Ticker
	mask        int32
	//usedBytes   int64
	//maxBytes    int64
}

func newLRU2Cache(opts Options) *lru2Store {
	if opts.BucketCount == 0 {
		opts.BucketCount = 16
	}
	if opts.CapPerBucket == 0 {
		opts.CapPerBucket = 128
	}
	if opts.Level2Cap == 0 {
		opts.Level2Cap = 64
	}

	mask := maskOfNextPowOf2(opts.BucketCount)
	s := &lru2Store{
		locks:     make([]sync.Mutex, mask+1),
		caches:    make([][2]*cache, mask+1),
		onEvicted: opts.OnEvicted,
		//maxBytes:    opts.MaxBytes,
		cleanupTick: time.NewTicker(opts.CleanupInterval),
		mask:        int32(mask),
	}

	for i := range s.caches {
		s.caches[i][0] = create(opts.CapPerBucket)
		s.caches[i][1] = create(opts.Level2Cap)
	}

	//cache.Inspect(func(action int, key string, iface *interface{}, bytes []byte, status int) {
	//	if action == ecache.DEL && status == 1 && s.onEvicted != nil && iface != nil {
	//		if value, ok := (*iface).(Value); ok {
	//			s.mu.Lock()
	//			s.usedBytes -= int64(len(key) + value.Len())
	//			s.mu.Unlock()
	//			s.onEvicted(key, value)
	//		}
	//	}
	//})

	if opts.CleanupInterval > 0 {
		s.cleanupTick = time.NewTicker(opts.CleanupInterval)
		go s.cleanupLoop()
	}

	return s
}

func (s *lru2Store) _get(key string, idx, level int32) (*node, int) {
	if n, st := s.caches[idx][level].get(key); st > 0 && n.expireAt > 0 && (n.expireAt == 0 || now() < n.expireAt) {
		return n, st
	}

	return nil, 0
}

func (s *lru2Store) Get(key string) (Value, bool) {
	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	if s.isExpired(key) {
		s.delete(key, idx)
		return nil, false
	}

	n, status := (*node)(nil), 0

	var expireAt int64
	if n, status, expireAt = s.caches[idx][0].del(key); status < 0 {
		n, status = s._get(key, idx, 1)
	} else {
		s.caches[idx][1].put(key, n.v, expireAt, s.onEvicted)
	}

	if status <= 0 || n == nil || n.expireAt <= 0 || (n.expireAt > 0 && now() >= n.expireAt) {
		return nil, false
	}

	return n.v, true
}

func (s *lru2Store) Set(key string, value Value) error {
	return s.SetWithExpiration(key, value, 0)
}

// SetWithExpiration 实现Store接口
func (s *lru2Store) SetWithExpiration(key string, value Value, expiration time.Duration) error {
	expireAt := int64(0)
	if expiration > 0 {
		expireAt = now() + int64(expiration)
		s.expirations.Store(key, time.Now().Add(expiration))
	} else {
		s.expirations.Delete(key)
	}

	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	s.caches[idx][0].put(key, value, expireAt, s.onEvicted)

	return nil
}

// Delete 实现Store接口
func (s *lru2Store) Delete(key string) bool {
	idx := hashBKRD(key) & s.mask
	s.locks[idx].Lock()
	defer s.locks[idx].Unlock()

	return s.delete(key, idx)
}

func (s *lru2Store) delete(key string, idx int32) bool {
	n1, s1, _ := s.caches[idx][0].del(key)
	n2, s2, _ := s.caches[idx][1].del(key)
	deleted := s1 > 0 || s2 > 0

	if deleted && s.onEvicted != nil {
		if n1 != nil && n1.v != nil {
			s.onEvicted(key, n1.v)
		} else if n2 != nil && n2.v != nil {
			s.onEvicted(key, n2.v)
		}
	}

	if deleted {
		s.expirations.Delete(key)
	}

	return deleted
}

// Clear 实现Store接口
func (s *lru2Store) Clear() {
	var keys []string

	for i := range s.caches {
		s.locks[i].Lock()

		s.caches[i][0].walk(func(key string, value Value, expireAt int64) bool {
			keys = append(keys, key)
			return true
		})

		s.caches[i][1].walk(func(key string, value Value, expireAt int64) bool {
			// 检查键是否已经收集（避免重复）
			for _, k := range keys {
				if key == k {
					return true
				}
			}

			keys = append(keys, key)
			return true
		})
		s.locks[i].Unlock()

	}

	for _, key := range keys {
		s.Delete(key)
	}

	s.expirations = sync.Map{}

}

// Len 实现Store接口
func (s *lru2Store) Len() int {
	count := 0

	for i := range s.caches {
		s.locks[i].Lock()

		s.caches[i][0].walk(func(key string, value Value, expireAt int64) bool {
			count++
			return true
		})

		s.caches[i][1].walk(func(key string, value Value, expireAt int64) bool {
			count++
			return true
		})

		s.locks[i].Unlock()
	}

	return count
}

// cleanupLoop 定期清理过期缓存
func (s *lru2Store) cleanupLoop() {
	for range s.cleanupTick.C {
		var expiredKeys []string

		s.expirations.Range(func(key, expireTime any) bool {
			if time.Now().After(expireTime.(time.Time)) {
				expiredKeys = append(expiredKeys, key.(string))
			}
			return true
		})

		for _, key := range expiredKeys {
			s.Delete(key)
		}
	}
}

// 检查键是否过期
func (s *lru2Store) isExpired(key string) bool {
	if expireTime, ok := s.expirations.Load(key); ok {
		if time.Now().After(expireTime.(time.Time)) {
			return true
		}
	}

	return false
}

// Close 关闭缓存相关资源
func (s *lru2Store) Close() {
	if s.cleanupTick != nil {
		s.cleanupTick.Stop()
	}
}
