package network

import (
	"sync"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/core" 
)

type Network struct {
	mu    sync.RWMutex
	nodes map[string]chan<- *core.Block
	
	Delta time.Duration
}

func NewNetwork(delta time.Duration) *Network {
	return &Network{
		nodes: make(map[string]chan<- *core.Block),
		Delta: delta,
	}
}

func (n *Network) Register(nodeID string, inbound chan<- *core.Block) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nodes[nodeID] = inbound
}

func (n *Network) Unregister(nodeID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.nodes, nodeID)
}

func (n *Network) Broadcast(block *core.Block, senderID string) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for id, ch := range n.nodes {
		if id == senderID {
			continue
		}

		peerCh := ch

		deliver := func() {
			select {
			case peerCh <- block:
			default:
			}
		}

		if n.Delta <= 0 {
			deliver()
		} else {
			time.AfterFunc(n.Delta, deliver)
		}
	}
}