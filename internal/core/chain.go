package core

import (
	"errors"
	"sync"
)

const (
	// DifficultyEpoch is the number of blocks between difficulty adjustments.
	DifficultyEpoch = 3
	// TargetTimePerBlock is the target time per block in milliseconds.
	TargetTimePerBlock = 1000
	// EpochTargetDuration is the total target time for one epoch.
	EpochTargetDuration = DifficultyEpoch * TargetTimePerBlock
	// MaxAdjustmentFactor caps the difficulty adjustment to prevent extreme swings.
	// With a small epoch of 3 blocks, PoW variance is high — a tight clamp
	// prevents wild oscillation and helps convergence toward 1s/block.
	MaxAdjustmentFactor = 2
	// MinAdjustmentFactor is the floor for difficulty adjustment.
	MinAdjustmentFactor = 0.5
)

// BlockNode represents a node in the block tree, tracking ancestry and cumulative work.
type BlockNode struct {
	Hash           string
	Block          *Block
	Parent         *BlockNode
	Height         uint64
	CumulativeDiff float64
}

// BlockChain is a thread-safe tree of blocks implementing Nakamoto consensus.
type BlockChain struct {
	mu          sync.RWMutex
	nodes       map[string]*BlockNode
	heaviestTip *BlockNode
}

// NewBlockChain creates a new chain seeded with the given genesis block.
func NewBlockChain(genesis *Block) (*BlockChain, error) {
	bc := &BlockChain{
		nodes: make(map[string]*BlockNode),
	}
	err := bc.AddBlock(genesis)
	return bc, err
}

// AddBlock validates and inserts a block into the tree.
// Returns nil if the block was already present (idempotent).
func (bc *BlockChain) AddBlock(b *Block) error {
	hash := b.Hash()

	if !IsValidPoW(hash, b.Header.Difficulty) {
		return errors.New("invalid proof of work")
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Idempotent: skip if already known.
	if _, exists := bc.nodes[hash]; exists {
		return nil
	}

	var parentNode *BlockNode
	var height uint64
	var cumDiff float64

	if b.Header.Parent != "" {
		var exists bool
		parentNode, exists = bc.nodes[b.Header.Parent]
		if !exists {
			return errors.New("parent block not found in chain state")
		}
		height = parentNode.Height + 1
		cumDiff = parentNode.CumulativeDiff + b.Header.Difficulty
	} else {
		// Genesis block
		height = 0
		cumDiff = b.Header.Difficulty
	}

	node := &BlockNode{
		Hash:           hash,
		Block:          b,
		Parent:         parentNode,
		Height:         height,
		CumulativeDiff: cumDiff,
	}

	bc.nodes[hash] = node

	// Update heaviest tip (highest cumulative difficulty wins).
	if bc.heaviestTip == nil || node.CumulativeDiff > bc.heaviestTip.CumulativeDiff {
		bc.heaviestTip = node
	}

	return nil
}

// GetHeaviestChainTip returns the tip of the heaviest chain (thread-safe).
func (bc *BlockChain) GetHeaviestChainTip() *BlockNode {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.heaviestTip
}

// GetAllNodes returns a snapshot of all block nodes in the tree (thread-safe).
func (bc *BlockChain) GetAllNodes() map[string]*BlockNode {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	copy := make(map[string]*BlockNode, len(bc.nodes))
	for k, v := range bc.nodes {
		copy[k] = v
	}
	return copy
}

// CalculateNextDifficulty computes the difficulty for a new block building on parentHash.
// Difficulty adjusts every DifficultyEpoch blocks, clamped by [MinAdjustmentFactor, MaxAdjustmentFactor].
func (bc *BlockChain) CalculateNextDifficulty(parentHash string) (float64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	parent, exists := bc.nodes[parentHash]
	if !exists {
		return 0, errors.New("parent not found")
	}

	nextHeight := parent.Height + 1

	// Only adjust at epoch boundaries.
	if nextHeight%DifficultyEpoch != 0 {
		return parent.Block.Header.Difficulty, nil
	}

	// Walk back up to DifficultyEpoch steps to find the ancestor starting this epoch.
	ancestor := parent
	for i := 0; i < DifficultyEpoch; i++ {
		if ancestor.Parent == nil {
			break
		}
		ancestor = ancestor.Parent
	}

	actualDuration := parent.Block.Header.Timestamp - ancestor.Block.Header.Timestamp
	if actualDuration <= 0 {
		actualDuration = 1 // Prevent division by zero.
	}

	numIntervals := parent.Height - ancestor.Height
	if numIntervals == 0 {
		return parent.Block.Header.Difficulty, nil
	}

	targetDuration := int64(numIntervals) * TargetTimePerBlock

	// ratio = target_time / actual_time
	ratio := float64(targetDuration) / float64(actualDuration)

	// Clamp the adjustment factor to prevent extreme spikes/crashes.
	if ratio > MaxAdjustmentFactor {
		ratio = MaxAdjustmentFactor
	}
	if ratio < MinAdjustmentFactor {
		ratio = MinAdjustmentFactor
	}

	newDiff := parent.Block.Header.Difficulty * ratio

	// Absolute floor for difficulty.
	if newDiff < 1.0 {
		newDiff = 1.0
	}

	return newDiff, nil
}