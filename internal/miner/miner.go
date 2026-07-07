package miner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	return &Miner{
		ID:          id,
		MiningDelay: miningDelay,
		Chain:       chain,
		Inbound:     inbound,
		Outbound:    outbound,
	}
}

// Start launches the mining goroutine. It mines until ctx is cancelled.
func (m *Miner) Start(ctx context.Context) {
	go m.run(ctx)
}

func (m *Miner) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
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
