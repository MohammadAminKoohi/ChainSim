package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

type BlockHeader struct {
	Parent     string  `json:"parent"`
	Digest     string  `json:"digest"`
	Difficulty float64 `json:"difficulty"`
	Timestamp  int64   `json:"timestamp"`
	Nonce      uint64  `json:"nonce"`
}

type Block struct {
	Header BlockHeader `json:"header"`
	Body   string      `json:"body"`
}

func (b *Block) Hash() string {
	headerStr := fmt.Sprintf("%s|%s|%.8f|%d|%d",
		b.Header.Parent,
		b.Header.Digest,
		b.Header.Difficulty,
		b.Header.Timestamp,
		b.Header.Nonce,
	)

	hash := sha256.Sum256([]byte(headerStr))
	return hex.EncodeToString(hash[:])
}

func IsValidPoW(hash string, difficulty float64) bool {
	if difficulty <= 0 {
		return true
	}

	hashInt := new(big.Int)
	if _, ok := hashInt.SetString(hash, 16); !ok {
		return false
	}

	maxTarget := new(big.Int)
	maxTarget.Exp(big.NewInt(2), big.NewInt(256), nil)
	maxTarget.Sub(maxTarget, big.NewInt(1))

	targetFloat := new(big.Float).SetInt(maxTarget)
	diffFloat := big.NewFloat(difficulty)

	targetFloat.Quo(targetFloat, diffFloat)

	targetInt := new(big.Int)
	targetFloat.Int(targetInt)

	return hashInt.Cmp(targetInt) <= 0
}