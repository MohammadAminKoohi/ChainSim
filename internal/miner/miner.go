package miner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/core"
)

// TipUpdateCallback is called when a miner's view of the heaviest tip changes.
type TipUpdateCallback func(minerID, oldTipHash, newTipHash string, height uint64)

// BlockMinedCallback is called when a miner successfully mines a block.
type BlockMinedCallback func(minerID, hash string, difficulty float64, height uint64)

// Miner simulates a PoW miner running as a goroutine.
type Miner struct {
	ID         string
	MiningDelay time.Duration // Micro-delay per hash attempt (controls hashrate).

	Chain    *core.BlockChain
	Inbound  <-chan *core.Block // Receives blocks from the network.
	Outbound chan<- *core.Block // Sends mined blocks to the router.

	// Optional telemetry callbacks.
	OnTipUpdated  TipUpdateCallback
	OnBlockMined  BlockMinedCallback

	miningEnabled atomic.Bool
	IsSelfish     bool
	privateBlocks []*core.Block
	MinedCount    uint64
}

// NewMiner creates a new miner with the given configuration.
// miningDelay controls hashrate: shorter delay = faster miner.
// Example: 10µs delay is 3x faster than 30µs delay.
func NewMiner(
	id string,
	miningDelay time.Duration,
	chain *core.BlockChain,
	inbound <-chan *core.Block,
	outbound chan<- *core.Block,
) *Miner {
	m := &Miner{
		ID:          id,
		MiningDelay: miningDelay,
		Chain:       chain,
		Inbound:     inbound,
		Outbound:    outbound,
	}
	m.miningEnabled.Store(true)
	return m
}

// StopMining disables the mining nonce-searching loop, but leaves the miner goroutine running to keep processing inbound blocks.
func (m *Miner) StopMining() {
	m.miningEnabled.Store(false)
}

// Start launches the mining goroutine. It mines until ctx is cancelled.
func (m *Miner) Start(ctx context.Context) {
	if m.IsSelfish {
		go m.runSelfish(ctx)
	} else {
		go m.run(ctx)
	}
}

func (m *Miner) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !m.miningEnabled.Load() {
			// Mining is disabled, but we must still process incoming network blocks to keep our chain synchronized.
			select {
			case <-ctx.Done():
				return
			case incomingBlock := <-m.Inbound:
				_ = m.Chain.AddBlock(incomingBlock)
			}
			continue
		}

		tip := m.Chain.GetHeaviestChainTip()
		var parentHash string
		var currentHeight uint64
		var parentTimestamp int64
		if tip != nil {
			parentHash = tip.Hash
			currentHeight = tip.Height
			parentTimestamp = tip.Block.Header.Timestamp
		} else {
			parentTimestamp = time.Now().UnixMilli()
		}

		diff, err := m.Chain.CalculateNextDifficulty(parentHash)
		if err != nil {
			diff = 1.0
		}

		bodyStr := fmt.Sprintf("Mined by %s at height %d", m.ID, currentHeight+1)
		digestHash := sha256.Sum256([]byte(bodyStr))

		block := &core.Block{
			Header: core.BlockHeader{
				Parent:     parentHash,
				Digest:     hex.EncodeToString(digestHash[:]),
				Difficulty: diff,
				Nonce:      0,
			},
			Body: bodyStr,
		}

		found := m.mineBlock(ctx, block, diff, parentHash, parentTimestamp)
		if !found {
			// Either ctx was cancelled or a new tip arrived — restart outer loop.
			continue
		}

		// Successfully mined — add to our chain and broadcast.
		if err := m.Chain.AddBlock(block); err != nil {
			continue
		}
		m.MinedCount++

		hash := block.Hash()
		newTip := m.Chain.GetHeaviestChainTip()

		// Fire telemetry callback for the mined block.
		if m.OnBlockMined != nil && newTip != nil {
			m.OnBlockMined(m.ID, hash, diff, newTip.Height)
		}

		// Send a COPY to the outbound channel so we don't share pointers.
		blockCopy := block.Copy()
		select {
		case m.Outbound <- blockCopy:
		case <-ctx.Done():
			return
		}
	}
}

// mineBlock runs the inner hashing loop. Returns true if a valid nonce was found,
// false if interrupted by context cancellation or a new network block.
func (m *Miner) mineBlock(ctx context.Context, block *core.Block, diff float64, parentHash string, parentTimestamp int64) bool {
	// Determine an appropriate batch size to ensure:
	// 1. We don't sleep too frequently (avoid OS timer overhead).
	// 2. We check the Inbound channel frequently enough (avoid propagation lag).
	// Let's target a batch duration of ~2ms.
	effectiveDelay := m.MiningDelay
	if effectiveDelay <= 0 {
		effectiveDelay = time.Microsecond
	}

	batchSize := uint64(1)
	if m.MiningDelay > 0 {
		targetBatchDuration := 2 * time.Millisecond
		batchSize = uint64(targetBatchDuration / m.MiningDelay)
		if batchSize < 1 {
			batchSize = 1
		}
	} else {
		batchSize = 1000 // Batch size for zero-delay mining to avoid excessive channel checks.
	}

	for {
		select {
		case <-ctx.Done():
			return false

		default:
			// Drain all incoming blocks from the network.
			hasNewTip := false
			for {
				select {
				case incomingBlock := <-m.Inbound:
					_ = m.Chain.AddBlock(incomingBlock)
					hasNewTip = true
				default:
					goto doneProcessing
				}
			}
		doneProcessing:

			if hasNewTip {
				newTip := m.Chain.GetHeaviestChainTip()
				if newTip != nil && newTip.Hash != parentHash {
					// The tip changed — fire the tip_updated callback and restart.
					if m.OnTipUpdated != nil {
						m.OnTipUpdated(m.ID, parentHash, newTip.Hash, newTip.Height)
					}
					return false
				}
			}

			// Mine a batch of nonces.
			for i := uint64(0); i < batchSize; i++ {
				// Calculate simulated timestamp based on the work done (Nonce * effectiveDelay).
				elapsedMilli := int64(float64(block.Header.Nonce) * float64(effectiveDelay) / float64(time.Millisecond))
				block.Header.Timestamp = parentTimestamp + elapsedMilli

				hash := block.Hash()
				if core.IsValidPoW(hash, diff) {
					return true
				}

				block.Header.Nonce++
			}

			if m.MiningDelay > 0 {
				time.Sleep(m.MiningDelay * time.Duration(batchSize))
			}
		}
	}
}

func (m *Miner) runSelfish(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !m.miningEnabled.Load() {
			// Mining is disabled, but we must still process incoming network blocks to keep our chain synchronized.
			select {
			case <-ctx.Done():
				return
			case incomingBlock := <-m.Inbound:
				_ = m.Chain.AddBlock(incomingBlock)
			}
			continue
		}

		tip := m.Chain.GetHeaviestChainTip()
		var parentHash string
		var parentTimestamp int64
		var currentHeight uint64

		if len(m.privateBlocks) > 0 {
			lastPrivate := m.privateBlocks[len(m.privateBlocks)-1]
			parentHash = lastPrivate.Hash()
			parentTimestamp = lastPrivate.Header.Timestamp
			if node, exists := m.Chain.GetAllNodes()[parentHash]; exists {
				currentHeight = node.Height
			}
		} else if tip != nil {
			parentHash = tip.Hash
			parentTimestamp = tip.Block.Header.Timestamp
			currentHeight = tip.Height
		} else {
			parentTimestamp = time.Now().UnixMilli()
		}

		diff, err := m.Chain.CalculateNextDifficulty(parentHash)
		if err != nil {
			diff = 1.0
		}

		bodyStr := fmt.Sprintf("Mined by %s at height %d", m.ID, currentHeight+1)
		digestHash := sha256.Sum256([]byte(bodyStr))

		block := &core.Block{
			Header: core.BlockHeader{
				Parent:     parentHash,
				Digest:     hex.EncodeToString(digestHash[:]),
				Difficulty: diff,
				Nonce:      0,
			},
			Body: bodyStr,
		}

		found := m.mineBlockSelfish(ctx, block, diff, parentHash, parentTimestamp)
		if !found {
			// Either ctx was cancelled or a new tip arrived and we aborted — restart outer loop.
			continue
		}

		// Successfully mined a private block!
		// Add to our local chain database.
		if err := m.Chain.AddBlock(block); err != nil {
			continue
		}
		m.MinedCount++

		m.privateBlocks = append(m.privateBlocks, block)
	}
}

func (m *Miner) mineBlockSelfish(ctx context.Context, block *core.Block, diff float64, parentHash string, parentTimestamp int64) bool {
	effectiveDelay := m.MiningDelay
	if effectiveDelay <= 0 {
		effectiveDelay = time.Microsecond
	}

	batchSize := uint64(1)
	if m.MiningDelay > 0 {
		targetBatchDuration := 2 * time.Millisecond
		batchSize = uint64(targetBatchDuration / m.MiningDelay)
		if batchSize < 1 {
			batchSize = 1
		}
	} else {
		batchSize = 1000
	}

	for {
		select {
		case <-ctx.Done():
			return false

		default:
			// Process all pending incoming blocks from the network.
			var incomingBlocks []*core.Block
			for {
				select {
				case incomingBlock := <-m.Inbound:
					incomingBlocks = append(incomingBlocks, incomingBlock)
				default:
					goto doneProcessing
				}
			}
		doneProcessing:

			if len(incomingBlocks) > 0 {
				hasNewTip := false
				for _, ib := range incomingBlocks {
					// Add to our local chain database.
					_ = m.Chain.AddBlock(ib)

					// Update our state based on the new honest block.
					// We only care if the new block is mined by the honest network (not ourselves).
					if !strings.Contains(ib.Body, m.ID) {
						hasNewTip = true
						m.handleHonestBlockMined(ib)
					}
				}

				if hasNewTip {
					newTip := m.Chain.GetHeaviestChainTip()
					var privateTipHash string
					if len(m.privateBlocks) > 0 {
						privateTipHash = m.privateBlocks[len(m.privateBlocks)-1].Hash()
					}

					if newTip != nil && newTip.Hash != parentHash && newTip.Hash != privateTipHash {
						// The public tip is now different from our mining parent and our private tip.
						// Abort and restart.
						return false
					}
				}
			}

			// Mine a batch of nonces.
			for i := uint64(0); i < batchSize; i++ {
				elapsedMilli := int64(float64(block.Header.Nonce) * float64(effectiveDelay) / float64(time.Millisecond))
				block.Header.Timestamp = parentTimestamp + elapsedMilli

				hash := block.Hash()
				if core.IsValidPoW(hash, diff) {
					return true
				}

				block.Header.Nonce++
			}

			if m.MiningDelay > 0 {
				time.Sleep(m.MiningDelay * time.Duration(batchSize))
			}
		}
	}
}

func (m *Miner) handleHonestBlockMined(ib *core.Block) {
	lA := len(m.privateBlocks)
	if lA == 0 {
		return
	}

	allNodes := m.Chain.GetAllNodes()
	honestNode, exists := allNodes[ib.Hash()]
	if !exists {
		return
	}

	firstPrivateHash := m.privateBlocks[0].Hash()
	firstPrivateNode, exists := allNodes[firstPrivateHash]
	if !exists {
		return
	}
	if firstPrivateNode.Parent == nil {
		return
	}
	forkHeight := firstPrivateNode.Parent.Height

	lH := honestNode.Height - forkHeight
	d := int(lA) - int(lH)

	if d < 0 {
		// Overtaken: discard private chain
		m.privateBlocks = nil
	} else if d == 0 {
		// Lead became 0 (race state): publish our 1 private block
		m.publishBlock(m.privateBlocks[0], "[lead=1]")
		m.privateBlocks = nil
	} else if d == 1 && lA == 2 {
		// Lead became 1 (from 2): publish both blocks
		m.publishBlock(m.privateBlocks[0], "[lead>=2]")
		m.publishBlock(m.privateBlocks[1], "[lead>=2]")
		m.privateBlocks = nil
	} else if d >= 2 {
		// Lead is still >= 2: publish blocks up to the height of the honest block
		for i := 0; i < int(lH); i++ {
			m.publishBlock(m.privateBlocks[i], "[lead>=2]")
		}
		m.privateBlocks = m.privateBlocks[lH:]
	}
}

func (m *Miner) publishBlock(block *core.Block, tag string) {
	if !strings.Contains(block.Body, tag) {
		block.Body = block.Body + " " + tag
	}

	blockCopy := block.Copy()
	select {
	case m.Outbound <- blockCopy:
	default:
	}
}
