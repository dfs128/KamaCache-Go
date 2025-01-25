package lc_cache

import (
	"github.com/juguagua/lc-cache/consistenthash"
	"sync"
)

// PeerPicker must be implemented to locate the peer that owns a specific key
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool, isSelf bool)
}

// PeerGetter must be implemented by a peer
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
	Delete(group string, key string) (bool, error)
}

type ClientPicker struct {
	self        string // self ip
	serviceName string
	mu          sync.RWMutex        // guards
	consHash    *consistenthash.Map // stores the list of peers, selected by specific key
}
