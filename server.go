package lcache

import (
	"context"
	"fmt"
	pb "github.com/juguagua/lcache/pb"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"net"
	"strings"
	"sync"
)

const (
	defaultSvcName = "lcache"
	defaultAddr    = "localhost:9999"
)

type Server struct {
	pb.UnimplementedLCacheServer
	svcName string // service name
	svcAddr string // service addr
	status  bool   // service status
	mu      sync.Mutex
	stopCh  chan error
}

type ServerOptions func(server *Server)

func NewServer(addr string, opts ...ServerOptions) (*Server, error) {
	if addr == "" {
		addr = defaultAddr
	}
	if !ValidPeerAddr(addr) {
		logrus.Errorf("invalid addr: %s", addr)
		return nil, fmt.Errorf("invalid addr: %s", addr)
	}
	server := &Server{
		svcAddr: addr,
		svcName: defaultSvcName,
	}
	for _, opt := range opts {
		opt(server)
	}

	return server, nil
}

func (s *Server) Get(ctx context.Context, in *pb.Request) (*pb.ResponseForGet, error) {
	group, key := in.GetGroup(), in.GetKey()
	logrus.Infof("lcache %s receive rpc requset, group: %s, key: %s", s.svcAddr, group, key)

	if key == "" {
		return nil, fmt.Errorf("key is empty")
	}
	g := GetGroup(group)
	if g == nil {
		return nil, fmt.Errorf("group %s not exist", group)
	}

	view, err := g.Get(key)
	if err != nil {
		logrus.Errorf("lcache %s get key %s error: %v", s.svcAddr, key, err)
		return nil, err
	}
	return &pb.ResponseForGet{
		Value: view.ByteSLice(),
	}, nil
}

func (s *Server) Delete(ctx context.Context, in *pb.Request) (*pb.ResponseForDelete, error) {
	group, key := in.GetGroup(), in.GetKey()
	logrus.Infof("lcache %s receive rpc requset, group: %s, key: %s", s.svcAddr, group, key)

	if key == "" {
		return nil, fmt.Errorf("key is empty")
	}
	g := GetGroup(group)
	if g == nil {
		return nil, fmt.Errorf("group %s not exist", group)
	}

	success, err := g.Delete(key)
	if err != nil {
		logrus.Errorf("lcache %s delete key %s error: %v", s.svcAddr, key, err)
		return nil, err
	}
	return &pb.ResponseForDelete{
		Value: success,
	}, nil
}

func (s *Server) Run() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status {
		return fmt.Errorf("server is running")
	}
	s.status = true
	s.stopCh = make(chan error)

	port := strings.Split(s.svcAddr, ":")[1]
	_, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return fmt.Errorf("listen %s error: %v", fmt.Sprintf("%s:%s", s.svcAddr, port), err)
	}

	server := grpc.NewServer()
	pb.RegisterLCacheServer(server, s)

	//go func() {
	//
	//}
	return nil
}

func (s *Server) Stop() {

}
