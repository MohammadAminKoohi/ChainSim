package core

import (
	"errors"
	"sync"
)

const (
	DifficultyEpoch = 3
	TargetTimePerBlock = 1000
	EpochTargetDuration = DifficultyEpoch * TargetTimePerBlock
)

type BlockNode struct {
	Hash           string
	Block          *Block
	Parent         *BlockNode
	Height         uint64
	CumulativeDiff float64
}

type BlockChain struct {
	mu          sync.RWMutex
	nodes       map[string]*BlockNode
	heaviestTip *BlockNode
}

func NewBlockChain(genesis *Block) (*BlockChain, error) {
	bc := &BlockChain{
		nodes: make(map[string]*BlockNode),
	}
	err := bc.AddBlock(genesis)
	return bc, err
}

func (bc *BlockChain) AddBlock(b *Block) error {
	hash := b.Hash()

	if !IsValidPoW(hash, b.Header.Difficulty) {
		return errors.New("invalid proof of work")
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

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

	if bc.heaviestTip == nil || node.CumulativeDiff > bc.heaviestTip.CumulativeDiff {
		bc.heaviestTip = node
	}

	return nil
}

func (bc *BlockChain) GetHeaviestChainTip() *BlockNode {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.heaviestTip
}

func (bc *BlockChain) CalculateNextDifficulty(parentHash string) (float64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	parent, exists := bc.nodes[parentHash]
	if !exists {
		return 0, errors.New("parent not found")
	}

	nextHeight := parent.Height + 1

	if nextHeight%DifficultyEpoch != 0 {
		return parent.Block.Header.Difficulty, nil
	}

	ancestor := parent
	for i := 0; i < DifficultyEpoch; i++ {
		if ancestor.Parent == nil {
			return parent.Block.Header.Difficulty, nil 
		}
		ancestor = ancestor.Parent
	}

	actualDuration := parent.Block.Header.Timestamp - ancestor.Block.Header.Timestamp
	
	if actualDuration <= 0 {
		actualDuration = 1 
	}

	ratio := float64(EpochTargetDuration) / float64(actualDuration)
	newDiff := parent.Block.Header.Difficulty * ratio

	if newDiff < 1.0 {
		newDiff = 1.0
	}

	return newDiff, nil
}