package lcache

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/juguagua/lcache/singleflight"
	"github.com/sirupsen/logrus"
)

var (
	lock   sync.RWMutex
	groups = sync.Map{}
)

// Getter 加载键值的回调函数接口
type Getter interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// GetterFunc 函数类型实现 Getter 接口
type GetterFunc func(ctx context.Context, key string) ([]byte, error)

func (f GetterFunc) Get(ctx context.Context, key string) ([]byte, error) {
	return f(ctx, key)
}

// Group 是一个缓存命名空间
type Group struct {
	name       string
	getter     Getter
	mainCache  cache
	peers      PeerPicker
	loadGroup  sync.WaitGroup // 防止缓存击穿
	loadMap    sync.Map       // key -> *call
	expiration time.Duration  // 缓存过期时间，0表示永不过期
	loader     *singleflight.Group
}

// call 正在进行中的请求
type call struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

// NewGroup 创建一个新的 Group 实例
func NewGroup(name string, cacheBytes int64, getter Getter, opts ...Option) *Group {
	if getter == nil {
		panic("nil Getter")
	}

	lock.Lock()
	defer lock.Unlock()

	g := &Group{
		name:   name,
		getter: getter,
		mainCache: cache{
			cacheBytes: cacheBytes,
		},
		loader: &singleflight.Group{},
	}

	// 应用选项
	for _, opt := range opts {
		opt(g)
	}

	groups.Store(name, g)
	return g
}

// Option 定义Group的配置选项
type Option func(*Group)

// WithExpiration 设置缓存过期时间
func WithExpiration(d time.Duration) Option {
	return func(g *Group) {
		g.expiration = d
	}
}

// WithPeers 设置分布式节点
func WithPeers(peers PeerPicker) Option {
	return func(g *Group) {
		g.peers = peers
	}
}

func GetGroup(name string) *Group {
	if g, ok := groups.Load(name); ok {
		return g.(*Group)
	}

	return nil
}

// Get 从缓存获取数据
func (g *Group) Get(ctx context.Context, key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, errors.New("key is required")
	}

	// 从本地缓存获取
	if v, ok := g.mainCache.get(key); ok {
		logrus.Debugf("[LCache] hit local cache, group: %s, key: %s", g.name, key)
		return v, nil
	}

	// 尝试从其他节点获取或加载
	return g.load(ctx, key)
}

// Set 设置缓存值
func (g *Group) Set(ctx context.Context, key string, value []byte) error {
	if key == "" {
		return errors.New("key is required")
	}
	if len(value) == 0 {
		return errors.New("value is required")
	}

	// 检查是否是从其他节点同步过来的请求
	if ctx.Value("from_peer") != nil {
		// 如果是从其他节点同步来的，只更新本地缓存，不再向其他节点同步
		view := ByteView{b: cloneBytes(value)}
		if g.expiration > 0 {
			g.mainCache.addWithExpiration(key, view, time.Now().Add(g.expiration))
		} else {
			g.mainCache.add(key, view)
		}
		return nil
	}

	// 本地发起的请求，需要同步到其他节点
	view := ByteView{b: cloneBytes(value)}
	if g.expiration > 0 {
		g.mainCache.addWithExpiration(key, view, time.Now().Add(g.expiration))
	} else {
		g.mainCache.add(key, view)
	}

	// 如果是分布式模式，同步到其他节点
	if g.peers != nil {
		if peer, ok, self := g.peers.PickPeer(key); ok && !self {
			go func() {
				ctx := context.WithValue(context.Background(), "from_peer", true)
				if err := peer.Set(ctx, g.name, key, value); err != nil {
					logrus.Errorf("[LCache] failed to sync set to peer: %v", err)
				}
			}()
		}
	}

	return nil
}

// Delete 删除缓存值
func (g *Group) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("key is required")
	}

	g.mainCache.delete(key)

	// 如果是分布式模式，同步删除到其他节点
	if g.peers != nil {
		if peer, ok, self := g.peers.PickPeer(key); ok && !self {
			if _, err := peer.Delete(g.name, key); err != nil {
				logrus.Errorf("[LCache] failed to sync delete to peer: %v", err)
				return err
			}
		}
	}

	return nil
}

// Clear 清空缓存
func (g *Group) Clear() {
	g.mainCache.Clear()
}

// load 加载数据
func (g *Group) load(ctx context.Context, key string) (value ByteView, err error) {
	// 使用 loadGroup 确保并发请求只加载一次
	view, err := g.loadSingle(ctx, key)
	if err != nil {
		return view, err
	}

	// 设置到本地缓存
	if g.expiration > 0 {
		g.mainCache.addWithExpiration(key, view, time.Now().Add(g.expiration))
	} else {
		g.mainCache.add(key, view)
	}

	return view, nil
}

// loadSingle 确保并发请求只加载一次
func (g *Group) loadSingle(ctx context.Context, key string) (value ByteView, err error) {
	// 检查是否已有相同的请求在处理
	if c, ok := g.loadMap.Load(key); ok {
		call := c.(call)
		call.wg.Wait()
		return ByteView{b: call.val}, call.err
	}

	// 创建新的请求
	c := call{}
	c.wg.Add(1)
	g.loadMap.Store(key, c)
	defer func() {
		g.loadMap.Delete(key)
	}()

	// 尝试从远程节点获取
	if g.peers != nil {
		if peer, ok, self := g.peers.PickPeer(key); ok && !self {
			value, err := g.getFromPeer(ctx, peer, key)
			if err == nil {
				c.val = value.b
				c.err = nil
				c.wg.Done()
				return value, nil
			}
			logrus.Warnf("[LCache] failed to get from peer: %v", err)
		}
	}

	// 从数据源加载
	bytes, err := g.getter.Get(ctx, key)
	if err != nil {
		c.err = err
		c.wg.Done()
		return ByteView{}, err
	}

	value = ByteView{b: cloneBytes(bytes)}
	c.val = value.b
	c.wg.Done()
	return value, nil
}

// getFromPeer 从其他节点获取数据
func (g *Group) getFromPeer(ctx context.Context, peer Peer, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, fmt.Errorf("failed to get from peer: %v", err)
	}
	return ByteView{b: bytes}, nil
}

// RegisterPeers 注册PeerPicker
func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeers called more than once")
	}
	g.peers = peers
}

// Stats 返回缓存统计信息
func (g *Group) Stats() map[string]interface{} {
	return map[string]interface{}{
		"name":       g.name,
		"cacheSize":  g.mainCache.Len(),
		"expiration": g.expiration,
	}
}

// DestroyGroup 销毁指定名称的缓存组
func DestroyGroup(name string) {
	g := GetGroup(name)
	if g != nil {
		groups.Delete(name)
		log.Printf("Destroyed cache group [%s]", name)
	}
}
