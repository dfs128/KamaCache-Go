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
go get github.com/juguagua/kamacache
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

