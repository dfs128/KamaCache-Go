【代码随想录知识星球】项目分享-缓存系统（Go）

# KamaCache

## 核心特性

### 1. 分布式架构
- 基于 etcd 的服务注册与发现
- 一致性哈希实现负载均衡
- 节点自动发现和同步
- 支持动态扩缩容

### 2. 缓存功能
- 支持 LRU 缓存策略
- 可配置过期时间
- 支持批量操作
- 防止缓存击穿
- 支持按组划分缓存空间

### 3. 高性能设计
- 并发安全
- 异步数据同步
- 单飞机制避免缓存击穿
- gRPC 通信

## 快速开始

### 1. 安装
```bash
go get github.com/youngyangyang04/KamaCache-Go
```

### 2. 启动 etcd
```bash
# 使用 Docker 启动 etcd
docker run -d --name etcd \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.0 \
  etcd --advertise-client-urls http://0.0.0.0:2379 \
  --listen-client-urls http://0.0.0.0:2379
```

### 3. 运行实例

详情见测试 demo：[example/test.go](example/test.go)


### 4. 多节点部署
```bash
# 启动节点 A
go run example/test.go -port 8001 -node A

# 启动节点 B
go run example/test.go -port 8002 -node B

# 启动节点 C
go run example/test.go -port 8003 -node C
```

### 5. 测试结果

A 进程：

```bash
go run example/test.go -port 8001 -node A  
2025/04/08 10:16:36 [节点A] 启动，地址: :8001
INFO[0000] Created cache group [test] with cacheBytes=2097152, expiration=0s 
INFO[0000] [KamaCache] registered peers for group [test] 
2025/04/08 10:16:36 [节点A] 等待节点注册...
2025/04/08 10:16:36 [节点A] 开始启动服务...
INFO[0000] Server starting at :8001                     
INFO[0000] Service registered: kama-cache at 172.22.152.216:8001 
INFO[0000] Successfully created client for 172.22.152.216:8001 
INFO[0000] New service discovered at 172.22.152.216:8001 
INFO[0002] Successfully created client for 172.22.152.216:8002 
INFO[0002] New service discovered at 172.22.152.216:8002 

=== 节点A：设置本地数据 ===
INFO[0005] Cache initialized with type lru2, max bytes: 2097152 
节点A: 设置键 key_A 成功
2025/04/08 10:16:41 [节点A] 等待其他节点准备就绪...
INFO[0005] grpc set request resp: value:"这是节点A的数据"      
INFO[0006] Successfully created client for 172.22.152.216:8003 
INFO[0006] New service discovered at 172.22.152.216:8003 
2025/04/08 10:17:11 当前已发现的节点:
2025/04/08 10:17:11 - 172.22.152.216:8002
2025/04/08 10:17:11 - 172.22.152.216:8003
2025/04/08 10:17:11 - 172.22.152.216:8001

=== 节点A：获取本地数据 ===
直接查询本地缓存...
缓存统计: map[cache_closed:false cache_hit_rate:0 cache_hits:0 cache_initialized:true cache_misses:0 cache_size:2 closed:false expiration:0s loader_errors:0 loader_hits:0 loads:0 local_hits:0 local_misses:0 name:test peets:0 peer_misses:0]
项目有效，将其移至二级缓存
节点A: 获取本地键 key_A 成功: 这是节点A的数据

=== 节点A：尝试获取远程数据 key_B ===
2025/04/08 10:17:11 [节点A] 开始查找键 key_B 的远程节点
项目有效，将其移至二级缓存
节点A: 获取远程键 key_B 成功: 这是节点B的数据

=== 节点A：尝试获取远程数据 key_C ===
2025/04/08 10:17:11 [节点A] 开始查找键 key_C 的远程节点
节点A: 获取远程键 key_C 成功: 这是节点C的数据
```

B 进程：

```bash
go run example/test.go -port 8002 -node B
2025/04/08 10:16:39 [节点B] 启动，地址: :8002
INFO[0000] Successfully created client for 172.22.152.216:8001 
INFO[0000] Discovered service at 172.22.152.216:8001    
INFO[0000] Created cache group [test] with cacheBytes=2097152, expiration=0s 
INFO[0000] [KamaCache] registered peers for group [test] 
2025/04/08 10:16:39 [节点B] 等待节点注册...
2025/04/08 10:16:39 [节点B] 开始启动服务...
INFO[0000] Server starting at :8002                     
INFO[0000] Service registered: kama-cache at 172.22.152.216:8002 
INFO[0000] Successfully created client for 172.22.152.216:8002 
INFO[0000] New service discovered at 172.22.152.216:8002 
INFO[0003] Successfully created client for 172.22.152.216:8003 
INFO[0003] New service discovered at 172.22.152.216:8003 

=== 节点B：设置本地数据 ===
INFO[0005] Cache initialized with type lru2, max bytes: 2097152 
节点B: 设置键 key_B 成功
2025/04/08 10:16:44 [节点B] 等待其他节点准备就绪...
INFO[0005] grpc set request resp: value:"这是节点B的数据"      
项目有效，将其移至二级缓存
2025/04/08 10:17:14 当前已发现的节点:
2025/04/08 10:17:14 - 172.22.152.216:8001
2025/04/08 10:17:14 - 172.22.152.216:8002
2025/04/08 10:17:14 - 172.22.152.216:8003

=== 节点B：获取本地数据 ===
直接查询本地缓存...
缓存统计: map[cache_closed:false cache_hit_rate:1 cache_hits:1 cache_initialized:true cache_misses:0 cache_size:2 closed:false expiration:0s hit_rate:1 loader_errors:0 loader_hits:0 loads:0 local_hits:1 local_misses:0 naest peer_hits:0 peer_misses:0]
项目有效，将其移至二级缓存
节点B: 获取本地键 key_B 成功: 这是节点B的数据

=== 节点B：尝试获取远程数据 key_A ===
2025/04/08 10:17:14 [节点B] 开始查找键 key_A 的远程节点
节点B: 获取远程键 key_A 成功: 这是节点A的数据

=== 节点B：尝试获取远程数据 key_C ===
2025/04/08 10:17:14 [节点B] 开始查找键 key_C 的远程节点
节点B: 获取远程键 key_C 成功: 这是节点C的数据
```

C 进程：

```bash
go run example/test.go -port 8003 -node C
2025/04/08 10:16:42 [节点C] 启动，地址: :8003
INFO[0000] Successfully created client for 172.22.152.216:8001 
INFO[0000] Discovered service at 172.22.152.216:8001    
INFO[0000] Successfully created client for 172.22.152.216:8002 
INFO[0000] Discovered service at 172.22.152.216:8002    
INFO[0000] Created cache group [test] with cacheBytes=2097152, expiration=0s 
INFO[0000] [KamaCache] registered peers for group [test] 
2025/04/08 10:16:42 [节点C] 等待节点注册...
2025/04/08 10:16:42 [节点C] 开始启动服务...
INFO[0000] Server starting at :8003                     
INFO[0000] Service registered: kama-cache at 172.22.152.216:8003 
INFO[0000] Successfully created client for 172.22.152.216:8003 
INFO[0000] New service discovered at 172.22.152.216:8003 

=== 节点C：设置本地数据 ===
INFO[0005] Cache initialized with type lru2, max bytes: 2097152 
节点C: 设置键 key_C 成功
2025/04/08 10:16:47 [节点C] 等待其他节点准备就绪...
INFO[0005] grpc set request resp: value:"这是节点C的数据"      
2025/04/08 10:17:17 当前已发现的节点:
2025/04/08 10:17:17 - 172.22.152.216:8001
2025/04/08 10:17:17 - 172.22.152.216:8002
2025/04/08 10:17:17 - 172.22.152.216:8003

=== 节点C：获取本地数据 ===
直接查询本地缓存...
缓存统计: map[cache_closed:false cache_hit_rate:0 cache_hits:0 cache_initialized:true cache_misses:0 cache_size:1 closed:false expiration:0s loader_errors:0 loader_hits:0 loads:0 local_hits:0 local_misses:0 name:test peets:0 peer_misses:0]
项目有效，将其移至二级缓存
节点C: 获取本地键 key_C 成功: 这是节点C的数据

=== 节点C：尝试获取远程数据 key_A ===
2025/04/08 10:17:17 [节点C] 开始查找键 key_A 的远程节点
节点C: 获取远程键 key_A 成功: 这是节点A的数据

=== 节点C：尝试获取远程数据 key_B ===
2025/04/08 10:17:17 [节点C] 开始查找键 key_B 的远程节点
节点C: 获取远程键 key_B 成功: 这是节点B的数据
```


## 配置说明

### 服务器配置
```go
type ServerOptions struct {
    EtcdEndpoints []string      // etcd 端点
    DialTimeout   time.Duration // 连接超时
    MaxMsgSize    int          // 最大消息大小
}
```

### 缓存组配置
```go
group := kamacache.NewGroup("users", 2<<20, getter,
  kamacache.WithExpiration(time.Hour),    // 设置过期时间
)
```

## 使用示例

### 1. 设置缓存
```go
err := group.Set(ctx, "key", []byte("value"))
```

### 2. 获取缓存
```go
value, err := group.Get(ctx, "key")
```

### 3. 删除缓存
```go
err := group.Delete(ctx, "key")
```

## 注意事项

1. 确保 etcd 服务可用
2. 合理配置缓存容量和过期时间
3. 节点地址不要重复
4. 建议在生产环境配置 TLS

## 性能优化

1. 使用一致性哈希实现负载均衡
2. 异步数据同步减少延迟
3. 单飞机制避免缓存击穿
4. 支持批量操作提高吞吐量

## 贡献指南

欢迎提交 Issue 和 Pull Request。

## 许可证

MIT License

