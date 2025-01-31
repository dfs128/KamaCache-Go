package registry

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	resolver "google.golang.org/grpc/resolver"
)

// Config 定义etcd客户端配置
type Config struct {
	Endpoints   []string      // 集群地址
	DialTimeout time.Duration // 连接超时时间
}

// DefaultConfig 提供默认配置
var DefaultConfig = &Config{
	Endpoints:   []string{"localhost:2379"},
	DialTimeout: 5 * time.Second,
}

// ServiceRegistry 服务注册器
type ServiceRegistry struct {
	client  *clientv3.Client
	config  *Config
	leaseID clientv3.LeaseID
}

// NewServiceRegistry 创建服务注册器
func NewServiceRegistry(cfg *Config) (*ServiceRegistry, error) {
	if cfg == nil {
		cfg = DefaultConfig
	}
	// 创建 etcd 客户端
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}

	return &ServiceRegistry{
		client: cli,
		config: cfg,
	}, nil
}

// EtcdDial 从 etcd 集群选择一个实例与其建立 grpc 连接
func EtcdDial(c *clientv3.Client, service, target string) (*grpc.ClientConn, error) {
	em, err := endpoints.NewManager(c, service)
	if err != nil {
		return nil, fmt.Errorf("failed to create endpoint manager: %v", err)
	}

	// 创建 etcd resolver builder
	builder := &etcdResolverBuilder{
		client:  c,
		manager: em,
	}
	resolver.Register(builder)

	// 使用 grpc.NewClient 建立连接
	return grpc.NewClient(
		fmt.Sprintf("etcd:///%s/%s", service, target),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// etcdResolverBuilder 实现 resolver.Builder 接口
type etcdResolverBuilder struct {
	client  *clientv3.Client
	manager endpoints.Manager
}

func (b *etcdResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &etcdResolver{
		client:     b.client,
		manager:    b.manager,
		target:     target,
		cc:         cc,
		addrsStore: make(map[string]struct{}),
	}
	r.start()
	return r, nil
}

func (b *etcdResolverBuilder) Scheme() string {
	return "etcd"
}

// etcdResolver 实现 resolver.Resolver 接口
type etcdResolver struct {
	client     *clientv3.Client
	manager    endpoints.Manager
	target     resolver.Target
	cc         resolver.ClientConn
	addrsStore map[string]struct{}
}

func (r *etcdResolver) start() {
	endpoints, err := r.manager.List(context.Background())
	if err != nil {
		logrus.Errorf("failed to list endpoints: %v", err)
		return
	}

	addresses := make([]resolver.Address, 0, len(endpoints))
	for _, ep := range endpoints {
		addresses = append(addresses, resolver.Address{Addr: ep.Addr})
		r.addrsStore[ep.Addr] = struct{}{}
	}

	r.cc.UpdateState(resolver.State{Addresses: addresses})
}

func (r *etcdResolver) ResolveNow(resolver.ResolveNowOptions) {
	r.start()
}

func (r *etcdResolver) Close() {}

// Register 注册服务
func (sr *ServiceRegistry) Register(ctx context.Context, service, addr string) error {
	// 创建租约
	lease, err := sr.client.Grant(ctx, 3)
	if err != nil {
		return fmt.Errorf("failed to create lease: %v", err)
	}
	sr.leaseID = lease.ID

	// 注册服务
	manager, err := endpoints.NewManager(sr.client, service)
	if err != nil {
		return fmt.Errorf("failed to create endpoint manager: %v", err)
	}

	endpoint := fmt.Sprintf("%s/%s", service, addr)
	err = manager.AddEndpoint(ctx, endpoint, endpoints.Endpoint{Addr: addr}, clientv3.WithLease(sr.leaseID))
	if err != nil {
		return fmt.Errorf("failed to add endpoint: %v", err)
	}

	// 保持租约
	keepAliveCh, err := sr.client.KeepAlive(ctx, sr.leaseID)
	if err != nil {
		return fmt.Errorf("failed to keep alive lease: %v", err)
	}

	logrus.Infof("registered service %s at %s with leaseID %d", service, addr, sr.leaseID)

	// 处理租约续约
	go sr.keepAliveWatch(ctx, service, addr, keepAliveCh)

	return nil
}

// keepAliveWatch 监控租约续约
func (sr *ServiceRegistry) keepAliveWatch(ctx context.Context, service, addr string, keepAliveCh <-chan *clientv3.LeaseKeepAliveResponse) {
	for {
		select {
		case <-ctx.Done():
			logrus.Info("context cancelled, stopping service registry")
			sr.revokeLease(context.Background())
			return

		case resp, ok := <-keepAliveCh:
			if !ok {
				logrus.Warn("keep alive channel closed")
				sr.revokeLease(context.Background())
				return
			}
			logrus.Debugf("received keepalive response: %v", resp)
		}
	}
}

// revokeLease 撤销租约
func (sr *ServiceRegistry) revokeLease(ctx context.Context) {
	if sr.leaseID != 0 {
		if _, err := sr.client.Revoke(ctx, sr.leaseID); err != nil {
			logrus.Errorf("failed to revoke lease: %v", err)
		}
	}
}

// Close 关闭服务注册器
func (sr *ServiceRegistry) Close() error {
	if sr.client != nil {
		return sr.client.Close()
	}
	return nil
}

// Register 注册服务到etcd
func Register(svcName, addr string, stopCh <-chan error) error {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   DefaultConfig.Endpoints,
		DialTimeout: DefaultConfig.DialTimeout,
	})
	if err != nil {
		return err
	}
	defer cli.Close()

	// 创建租约
	lease, err := cli.Grant(context.Background(), 3)
	if err != nil {
		return err
	}

	// 注册服务
	key := fmt.Sprintf("%s/%s", svcName, addr)
	_, err = cli.Put(context.Background(), key, addr, clientv3.WithLease(lease.ID))
	if err != nil {
		return err
	}

	// 保持租约
	ch, err := cli.KeepAlive(context.Background(), lease.ID)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-stopCh:
				cli.Revoke(context.Background(), lease.ID)
				return
			case <-ch:
				// 租约续期成功
			}
		}
	}()

	return nil
}
