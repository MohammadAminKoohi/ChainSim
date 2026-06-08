package miner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"time"

	"github.com/MohammadAminKoohi/ChainSim/internal/core"
)

type Miner struct {
	ID        string
	Hashrate  float64
	BaseDelay time.Duration

	Chain    *core.BlockChain
	Inbound  <-chan *core.Block
	Outbound chan<- *core.Block
}

func NewMiner(id string, hashrate float64, baseDelay time.Duration, chain *core.BlockChain, inbound <-chan *core.Block, outbound chan<- *core.Block) *Miner {
	return &Miner{
		ID:        id,
		Hashrate:  hashrate,
		BaseDelay: baseDelay,
		Chain:     chain,
		Inbound:   inbound,
		Outbound:  outbound,
	}
}

func (m *Miner) Start(ctx context.Context) {
	go func() {
		var delayPerHash time.Duration
		if m.Hashrate > 0 {
			delayPerHash = time.Duration(float64(m.BaseDelay) / m.Hashrate)
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			tip := m.Chain.GetHeaviestChainTip()
			var parentHash string
			var currentHeight uint64
			if tip != nil {
				parentHash = tip.Hash
				currentHeight = tip.Height
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
					Digest:     hex.EncodeToString(digestHash[:]), // FIX: Cryptographically link the body
					Difficulty: diff,
					Nonce:      0,
				},
				Body: bodyStr,
			}

		MiningLoop:
			for {
				select {
				case <-ctx.Done():
					return

				case incomingBlock := <-m.Inbound:
					_ = m.Chain.AddBlock(incomingBlock)

					newTip := m.Chain.GetHeaviestChainTip()
					if newTip != nil && newTip.Hash != parentHash {
						break MiningLoop
					}

				default:
					block.Header.Timestamp = time.Now().UnixMilli()
					hash := block.Hash()

					if core.IsValidPoW(hash, diff) {
						_ = m.Chain.AddBlock(block)

						select {
						case m.Outbound <- block:
						case <-ctx.Done():
							return
						default:
						}

						break MiningLoop
					}

					block.Header.Nonce++

					if delayPerHash > 0 {
						time.Sleep(delayPerHash)
					} else {
						runtime.Gosched()
					}
				}
			}
		}
	}()
}
