【代码随想录知识星球】项目分享-缓存系统（Go）

# KamaCache

KamaCache 是一个高性能的分布式缓存系统，支持多种缓存策略和自动负载均衡。

## 特性

### 1. 多样化的缓存策略
- 支持 LRU (Least Recently Used) 缓存淘汰算法
- 支持 LFU (Least Frequently Used) 缓存淘汰算法（规划中）
- 可扩展的缓存策略接口，便于添加新的缓存算法

### 2. 分布式支持
- 基于 etcd 的服务注册与发现
- 一致性哈希实现负载均衡
- 支持节点的动态添加和删除
- 自动故障转移

### 3. 高性能设计
- 并发安全的缓存访问
- 使用 sync.Map 优化并发性能
- 支持批量操作
- 防止缓存击穿的单飞机制

### 4. 可靠性保证
- 支持 TLS 安全传输
- 健康检查机制
- 优雅关闭支持
- 完善的错误处理

### 5. 缓存功能
- 支持设置过期时间
- 支持 Get/Set/Delete 操作
- 支持批量清理
- 支持按组划分缓存空间

### 6. 负载均衡
- 动态虚拟节点调整
- 自动负载均衡
- 可配置的负载均衡阈值
- 负载统计和监控

### 7. 可观测性
- 详细的日志记录
- 性能统计信息
- 健康状态监控
- 支持自定义回调函数

### 8. 配置灵活
- 支持配置文件
- 运行时动态配置
- 多种配置选项
- 默认配置支持

## 快速开始

### 安装

```bash
go get github.com/juguagua/lcache
```

### 基本使用

```go
// 创建缓存组
group := lcache.NewGroup("users", 2<<10, lcache.GetterFunc(
    func(ctx context.Context, key string) ([]byte, error) {
        // 从数据源加载数据
        return []byte("value"), nil
    },
))

// 设置缓存
err := group.Set(ctx, "key1", []byte("value1"))

// 获取缓存
value, err := group.Get(ctx, "key1")

// 删除缓存
err = group.Delete(ctx, "key1")
```

### 启动服务器
```go
server, err := lcache.NewServer(":8001", "lcache",
    lcache.WithEtcdEndpoints([]string{"localhost:2379"}),
    lcache.WithDialTimeout(5*time.Second),
)
if err != nil {
    log.Fatal(err)
}

if err := server.Start(); err != nil {
    log.Fatal(err)
}
```

## 配置说明

### 缓存配置
- MaxBytes: 最大内存使用量
- CleanupInterval: 清理间隔
- DefaultExpiration: 默认过期时间

### 服务器配置
- EtcdEndpoints: etcd 服务地址
- DialTimeout: 连接超时时间
- MaxMsgSize: 最大消息大小
- TLS: TLS 配置

## 注意事项
1. 确保 etcd 服务可用
2. 合理配置内存限制
3. 适当设置过期时间
4. 注意配置负载均衡参数

## 贡献指南
欢迎提交 Issue 和 Pull Request

## 许可证
MIT License

