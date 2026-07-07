package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// BlockHeader represents the header of a block in the blockchain.
type BlockHeader struct {
	Parent     string  `json:"parent"`
	Digest     string  `json:"digest"`
	Difficulty float64 `json:"difficulty"`
	Timestamp  int64   `json:"timestamp"`
	Nonce      uint64  `json:"nonce"`
}

// Block represents a full block with header and body.
type Block struct {
	Header BlockHeader `json:"header"`
	Body   string      `json:"body"`
}

// Hash deterministically serializes the header and returns the SHA-256 hex digest.
func (b *Block) Hash() string {
	headerStr := fmt.Sprintf("%s|%s|%e|%d|%d",
		b.Header.Parent,
		b.Header.Digest,
		b.Header.Difficulty,
		b.Header.Timestamp,
		b.Header.Nonce,
	)
	hash := sha256.Sum256([]byte(headerStr))
	return hex.EncodeToString(hash[:])
}

// Copy returns a deep copy of the block, safe to send across goroutines.
func (b *Block) Copy() *Block {
	return &Block{
		Header: BlockHeader{
			Parent:     b.Header.Parent,
			Digest:     b.Header.Digest,
			Difficulty: b.Header.Difficulty,
			Timestamp:  b.Header.Timestamp,
			Nonce:      b.Header.Nonce,
		},
		Body: b.Body,
	}
}

// IsValidPoW checks whether the given hash meets the difficulty target.
// Target = (2^256 - 1) / difficulty. Hash must be <= target.
func IsValidPoW(hash string, difficulty float64) bool {
	if difficulty <= 0 {
		return true
	}

	hashInt := new(big.Int)
	if _, ok := hashInt.SetString(hash, 16); !ok {
		return false
	}

	// maxTarget = 2^256 - 1
	maxTarget := new(big.Int)
	maxTarget.Exp(big.NewInt(2), big.NewInt(256), nil)
	maxTarget.Sub(maxTarget, big.NewInt(1))

	// target = maxTarget / difficulty
	targetFloat := new(big.Float).SetInt(maxTarget)
	diffFloat := big.NewFloat(difficulty)
	targetFloat.Quo(targetFloat, diffFloat)

	targetInt := new(big.Int)
	targetFloat.Int(targetInt)

	return hashInt.Cmp(targetInt) <= 0
}