package network

import (
	"sync"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/core"
)

// Network is a centralized broker that simulates block propagation
// between miners using Go channels and configurable latency (Delta).
type Network struct {
	mu    sync.RWMutex
	nodes map[string]chan<- *core.Block

	// Delta is the simulated network latency applied to each broadcast.
	Delta time.Duration
}

// NewNetwork creates a network broker with the given propagation delay.
func NewNetwork(delta time.Duration) *Network {
	return &Network{
		nodes: make(map[string]chan<- *core.Block),
		Delta: delta,
	}
}

// Register adds a miner's inbound channel to the network.
func (n *Network) Register(nodeID string, inbound chan<- *core.Block) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nodes[nodeID] = inbound
}

// Unregister removes a miner from the network.
func (n *Network) Unregister(nodeID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.nodes, nodeID)
}

// Broadcast sends a deep-copy of the block to all registered peers except the sender.
// If Delta > 0, delivery is delayed by exactly Delta using time.AfterFunc.
// Sends are non-blocking: if a peer's channel buffer is full, the block is dropped
// for that peer (simulates a busy node missing a message).
func (n *Network) Broadcast(block *core.Block, senderID string) {
	n.mu.RLock()
	// Snapshot the peer channels under the lock, then release.
	peers := make(map[string]chan<- *core.Block, len(n.nodes))
	for id, ch := range n.nodes {
		if id != senderID {
			peers[id] = ch
		}
	}
	n.mu.RUnlock()

	for _, ch := range peers {
		// Deep-copy the block so each delivery is independent.
		// This prevents race conditions when Delta > 0 and the sender
		// mutates the block struct after broadcasting.
		blockCopy := block.Copy()
		peerCh := ch

		deliver := func() {
			select {
			case peerCh <- blockCopy:
			default:
				// Peer channel is full — drop this delivery.
			}
		}

		if n.Delta <= 0 {
			deliver()
		} else {
			time.AfterFunc(n.Delta, deliver)
		}
	}
}