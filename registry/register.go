package registry

import (
	"context"
	"fmt"
	"net"
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
	// 直接使用 grpc.Dial
	return grpc.Dial(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(3*time.Second),
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
		return fmt.Errorf("failed to create etcd client: %v", err)
	}

	localIP, err := getLocalIP()
	if err != nil {
		cli.Close()
		return fmt.Errorf("failed to get local IP: %v", err)
	}
	if addr[0] == ':' {
		addr = fmt.Sprintf("%s%s", localIP, addr)
	}

	// 创建租约
	lease, err := cli.Grant(context.Background(), 10) // 增加租约时间到10秒
	if err != nil {
		cli.Close()
		return fmt.Errorf("failed to create lease: %v", err)
	}

	// 注册服务，使用完整的key路径
	key := fmt.Sprintf("/services/%s/%s", svcName, addr)
	_, err = cli.Put(context.Background(), key, addr, clientv3.WithLease(lease.ID))
	if err != nil {
		cli.Close()
		return fmt.Errorf("failed to put key-value to etcd: %v", err)
	}

	// 保持租约
	keepAliveCh, err := cli.KeepAlive(context.Background(), lease.ID)
	if err != nil {
		cli.Close()
		return fmt.Errorf("failed to keep lease alive: %v", err)
	}

	// 处理租约续期和服务注销
	go func() {
		defer cli.Close()
		for {
			select {
			case <-stopCh:
				// 服务注销，撤销租约
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				cli.Revoke(ctx, lease.ID)
				cancel()
				return
			case resp, ok := <-keepAliveCh:
				if !ok {
					logrus.Warn("keep alive channel closed")
					return
				}
				logrus.Debugf("successfully renewed lease: %d", resp.ID)
			}
		}
	}()

	logrus.Infof("Service registered: %s at %s", svcName, addr)
	return nil
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no valid local IP found")
}
